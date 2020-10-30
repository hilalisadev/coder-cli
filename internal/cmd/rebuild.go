package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"cdr.dev/coder-cli/coder-sdk"
	"cdr.dev/coder-cli/internal/clog"
	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh/terminal"
	"golang.org/x/xerrors"
)

func rebuildEnvCommand(user *string) *cobra.Command {
	var follow bool
	var force bool
	cmd := &cobra.Command{
		Use:   "rebuild [environment_name]",
		Short: "rebuild a Coder environment",
		Args:  cobra.ExactArgs(1),
		Example: `coder envs rebuild front-end-env --follow
coder envs rebuild backend-env --force`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			client, err := newClient(ctx)
			if err != nil {
				return err
			}
			env, err := findEnv(ctx, client, args[0], *user)
			if err != nil {
				return err
			}

			if !force && env.LatestStat.ContainerStatus == coder.EnvironmentOn {
				_, err = (&promptui.Prompt{
					Label:     fmt.Sprintf("Rebuild environment \"%s\"? (will destroy any work outside of /home)", env.Name),
					IsConfirm: true,
				}).Run()
				if err != nil {
					return clog.Fatal(
						"failed to confirm prompt", clog.BlankLine,
						clog.Tipf(`use "--force" to rebuild without a confirmation prompt`),
					)
				}
			}

			if err = client.RebuildEnvironment(ctx, env.ID); err != nil {
				return err
			}
			if follow {
				if err = trailBuildLogs(ctx, client, env.ID); err != nil {
					return err
				}
			} else {
				clog.LogSuccess(
					"successfully started rebuild",
					clog.Tipf("run \"coder envs watch-build %s\" to follow the build logs", env.Name),
				)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&follow, "follow", false, "follow build log after initiating rebuild")
	cmd.Flags().BoolVar(&force, "force", false, "force rebuild without showing a confirmation prompt")
	return cmd
}

// trailBuildLogs follows the build log for a given environment and prints the staged
// output with loaders and success/failure indicators for each stage
func trailBuildLogs(ctx context.Context, client *coder.Client, envID string) error {
	const check = "✅"
	const failure = "❌"

	newSpinner := func() *spinner.Spinner { return spinner.New(spinner.CharSets[11], 100*time.Millisecond) }

	// this tells us whether to show dynamic loaders when printing output
	isTerminal := terminal.IsTerminal(int(os.Stdout.Fd()))

	logs, err := client.FollowEnvironmentBuildLog(ctx, envID)
	if err != nil {
		return err
	}
	var s *spinner.Spinner
	for l := range logs {
		if l.Err != nil {
			return l.Err
		}
		switch l.BuildLog.Type {
		case coder.BuildLogTypeStart:
			// the FE uses this to reset the UI
			// the CLI doesn't need to do anything here given that we only append to the trail
		case coder.BuildLogTypeStage:
			msg := fmt.Sprintf("%s %s", l.BuildLog.Time.Format(time.RFC3339), l.BuildLog.Msg)
			if !isTerminal {
				fmt.Println(msg)
				continue
			}
			if s != nil {
				s.Stop()
				fmt.Print("\n")
			}
			s = newSpinner()
			s.Suffix = fmt.Sprintf("  -- %s", msg)
			s.FinalMSG = fmt.Sprintf("%s -- %s", check, msg)
			s.Start()
		case coder.BuildLogTypeSubstage:
			// TODO(@cmoog) add verbose substage printing
		case coder.BuildLogTypeError:
			errMsg := color.RedString("\t%s", l.BuildLog.Msg)
			if !isTerminal {
				fmt.Println(errMsg)
				continue
			}
			if s != nil {
				s.FinalMSG = fmt.Sprintf("%s %s", failure, strings.TrimPrefix(s.Suffix, "  "))
				s.Stop()
			}
			fmt.Print(errMsg)
			s = newSpinner()
		case coder.BuildLogTypeDone:
			if s != nil {
				s.Stop()
			}
			return nil
		default:
			return xerrors.Errorf("unknown buildlog type: %s", l.BuildLog.Type)
		}
	}
	return nil
}

func watchBuildLogCommand(user *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "watch-build [environment_name]",
		Example: "coder envs watch-build front-end-env",
		Short:   "trail the build log of a Coder environment",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			client, err := newClient(ctx)
			if err != nil {
				return err
			}
			env, err := findEnv(ctx, client, args[0], *user)
			if err != nil {
				return err
			}

			if err = trailBuildLogs(ctx, client, env.ID); err != nil {
				return err
			}
			return nil
		},
	}
	return cmd
}
