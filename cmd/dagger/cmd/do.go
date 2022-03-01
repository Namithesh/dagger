package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"cuelang.org/go/cue"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.dagger.io/dagger/cmd/dagger/cmd/common"
	"go.dagger.io/dagger/cmd/dagger/logger"
	"go.dagger.io/dagger/plan"
	"go.dagger.io/dagger/solver"
	"golang.org/x/term"
)

var doCmd = &cobra.Command{
	Use:   "do [OPTIONS] ACTION [SUBACTION...]",
	Short: "Execute a dagger action.",
	// Args:  cobra.MinimumNArgs(1),
	PreRun: func(cmd *cobra.Command, args []string) {
		// Fix Viper bug for duplicate flags:
		// https://github.com/spf13/viper/issues/233
		if err := viper.BindPFlags(cmd.Flags()); err != nil {
			panic(err)
		}
	},
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) < 1 {
			doHelp(cmd, nil)
			return
		}

		var (
			lg  = logger.New()
			tty *logger.TTYOutput
			err error
		)

		if f := viper.GetString("log-format"); f == "tty" || f == "auto" && term.IsTerminal(int(os.Stdout.Fd())) {
			tty, err = logger.NewTTYOutput(os.Stderr)
			if err != nil {
				lg.Fatal().Err(err).Msg("failed to initialize TTY logger")
			}
			tty.Start()
			defer tty.Stop()

			lg = lg.Output(tty)
		}

		ctx := lg.WithContext(cmd.Context())
		cl := common.NewClient(ctx)

		p, err := loadPlan(getTargetPath(args).String())
		if err != nil {
			lg.Fatal().Err(err).Msg("failed to load plan")
		}

		err = cl.Do(ctx, p.Context(), func(ctx context.Context, s solver.Solver) error {
			_, err := p.Up(ctx, s)
			if err != nil {
				return err
			}

			return nil
		})

		// FIXME: rework telemetry

		if err != nil {
			lg.Fatal().Err(err).Msg("failed to up environment")
		}
	},
}

func loadPlan(target string) (*plan.Plan, error) {
	project := viper.GetString("project")
	return plan.Load(context.Background(), plan.Config{
		Args:   []string{project},
		With:   viper.GetStringSlice("with"),
		Target: target,
		Vendor: !viper.GetBool("no-vendor"),
	})
}

func getTargetPath(args []string) cue.Path {
	actionLookupArgs := []string{plan.ActionsPath}
	actionLookupArgs = append(actionLookupArgs, args...)
	actionLookupSelectors := []cue.Selector{}
	for _, a := range actionLookupArgs {
		actionLookupSelectors = append(actionLookupSelectors, cue.Str(a))
	}
	return cue.MakePath(actionLookupSelectors...)
}

func doHelp(cmd *cobra.Command, _ []string) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', tabwriter.StripEscape)
	defer w.Flush()

	p, err := loadPlan("")
	if err != nil {
		fmt.Printf("%s", err)
		fmt.Fprintln(w, "failed to load plan")
		return
	}
	project := viper.GetString("project")
	actionLookupPath := getTargetPath(cmd.Flags().Args())
	actions := p.Action().FindByPath(actionLookupPath).Children

	fmt.Printf(`Execute a dagger action.
	
%s

Plan loaded from %s:
%s
`, cmd.UsageString(), project, "\n"+actionLookupPath.String()+":")

	// fmt.Fprintln(w, "Actions\tDescription\tPackage")
	// fmt.Fprintln(w, "\t\t")
	for _, a := range actions {
		if !a.Hidden {
			lineParts := []string{"", a.Name, strings.TrimSpace(a.Comment)}
			fmt.Fprintln(w, strings.Join(lineParts, "\t"))
		}
	}
}

func init() {
	doCmd.Flags().StringArrayP("with", "w", []string{}, "")
	doCmd.Flags().Bool("no-vendor", false, "Force up, disable inputs check")

	doCmd.SetHelpFunc(doHelp)

	if err := viper.BindPFlags(doCmd.Flags()); err != nil {
		panic(err)
	}
}
