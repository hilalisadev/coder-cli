package cmd

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"cdr.dev/coder-cli/coder-sdk"
	"cdr.dev/coder-cli/internal/clog"
	"cdr.dev/coder-cli/internal/x/xtabwriter"
)

func secretsCmd() *cobra.Command {
	var user string
	cmd := &cobra.Command{
		Use:   "secrets",
		Short: "Interact with Coder Secrets",
		Long:  "Interact with secrets objects owned by the active user.",
	}
	cmd.PersistentFlags().StringVar(&user, "user", coder.Me, "Specify the user whose resources to target")
	cmd.AddCommand(
		&cobra.Command{
			Use:   "ls",
			Short: "List all secrets owned by the active user",
			RunE:  listSecretsCmd(&user),
		},
		createSecretCmd(&user),
		&cobra.Command{
			Use:     "rm [...secret_name]",
			Short:   "Remove one or more secrets by name",
			Args:    cobra.MinimumNArgs(1),
			RunE:    removeSecretsCmd(&user),
			Example: "coder secrets rm mysql-password mysql-user",
		},
		&cobra.Command{
			Use:     "view [secret_name]",
			Short:   "View a secret by name",
			Args:    cobra.ExactArgs(1),
			RunE:    viewSecretCmd(&user),
			Example: "coder secrets view mysql-password",
		},
	)
	return cmd
}

func createSecretCmd(userEmail *string) *cobra.Command {
	var (
		fromFile    string
		fromLiteral string
		fromPrompt  bool
		description string
	)

	cmd := &cobra.Command{
		Use:   "create [secret_name]",
		Short: "Create a new secret",
		Long:  "Create a new secret object to store application secrets and access them securely from within your environments.",
		Example: `coder secrets create mysql-password --from-literal 123password
coder secrets create mysql-password --from-prompt
coder secrets create aws-credentials --from-file ./credentials.json`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return xerrors.Errorf("[secret_name] is a required argument")
			}
			if fromPrompt && (fromLiteral != "" || fromFile != "") {
				return xerrors.Errorf("--from-prompt cannot be set along with --from-file or --from-literal")
			}
			if fromLiteral != "" && fromFile != "" {
				return xerrors.Errorf("--from-literal and --from-file cannot both be set")
			}
			if !fromPrompt && fromFile == "" && fromLiteral == "" {
				return xerrors.Errorf("one of [--from-literal, --from-file, --from-prompt] is required")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			var (
				name  = args[0]
				value string
				err   error
				ctx   = cmd.Context()
			)
			client, err := newClient(ctx)
			if err != nil {
				return err
			}
			if fromLiteral != "" {
				value = fromLiteral
			} else if fromFile != "" {
				contents, err := ioutil.ReadFile(fromFile)
				if err != nil {
					return xerrors.Errorf("read file: %w", err)
				}
				value = string(contents)
			} else {
				prompt := promptui.Prompt{
					Label: "value",
					Mask:  '*',
					Validate: func(s string) error {
						if len(s) < 1 {
							return xerrors.Errorf("a length > 0 is required")
						}
						return nil
					},
				}
				value, err = prompt.Run()
				if err != nil {
					return xerrors.Errorf("prompt for value: %w", err)
				}
			}

			user, err := client.UserByEmail(ctx, *userEmail)
			if err != nil {
				return xerrors.Errorf("get user %q by email: %w", *userEmail, err)
			}
			err = client.InsertSecret(ctx, user, coder.InsertSecretReq{
				Name:        name,
				Value:       value,
				Description: description,
			})
			if err != nil {
				return xerrors.Errorf("insert secret: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&fromFile, "from-file", "", "a file from which to read the value of the secret")
	cmd.Flags().StringVar(&fromLiteral, "from-literal", "", "the value of the secret")
	cmd.Flags().BoolVar(&fromPrompt, "from-prompt", false, "enter the secret value through a terminal prompt")
	cmd.Flags().StringVar(&description, "description", "", "a description of the secret")

	return cmd
}

func listSecretsCmd(userEmail *string) func(cmd *cobra.Command, _ []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		ctx := cmd.Context()
		client, err := newClient(ctx)
		if err != nil {
			return err
		}
		user, err := client.UserByEmail(ctx, *userEmail)
		if err != nil {
			return xerrors.Errorf("get user %q by email: %w", *userEmail, err)
		}

		secrets, err := client.Secrets(ctx, user.ID)
		if err != nil {
			return xerrors.Errorf("get secrets: %w", err)
		}

		if len(secrets) < 1 {
			clog.LogInfo("no secrets found")
			return nil
		}

		err = xtabwriter.WriteTable(len(secrets), func(i int) interface{} {
			s := secrets[i]
			s.Value = "******" // value is omitted from bulk responses
			return s
		})
		if err != nil {
			return xerrors.Errorf("write table of secrets: %w", err)
		}
		return nil
	}
}

func viewSecretCmd(userEmail *string) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		var (
			name = args[0]
			ctx  = cmd.Context()
		)
		client, err := newClient(ctx)
		if err != nil {
			return err
		}
		user, err := client.UserByEmail(ctx, *userEmail)
		if err != nil {
			return xerrors.Errorf("get user %q by email: %w", *userEmail, err)
		}

		secret, err := client.SecretWithValueByName(ctx, name, user.ID)
		if err != nil {
			return xerrors.Errorf("get secret by name: %w", err)
		}

		_, err = fmt.Fprintln(os.Stdout, secret.Value)
		if err != nil {
			return xerrors.Errorf("write secret value: %w", err)
		}
		return nil
	}
}

func removeSecretsCmd(userEmail *string) func(c *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := newClient(ctx)
		if err != nil {
			return err
		}
		user, err := client.UserByEmail(ctx, *userEmail)
		if err != nil {
			return xerrors.Errorf("get user %q by email: %w", *userEmail, err)
		}

		errorSeen := false
		for _, n := range args {
			err := client.DeleteSecretByName(ctx, n, user.ID)
			if err != nil {
				clog.Log(clog.Error(
					fmt.Sprintf("failed to delete secret %q: %v", n, err),
				))
				errorSeen = true
			} else {
				clog.LogSuccess("successfully deleted secret: %q", n)
			}
		}
		if errorSeen {
			os.Exit(1)
		}
		return nil
	}
}
