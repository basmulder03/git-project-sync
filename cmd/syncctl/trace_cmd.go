package main

import "github.com/spf13/cobra"

func newTraceCommand(configPath *string) *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "trace",
		Short: "Query trace details",
	}

	showCmd := &cobra.Command{
		Use:   "show <trace-id>",
		Short: "Show events for one trace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireNonEmpty(args[0], "trace-id"); err != nil {
				return err
			}

			api, closer, err := loadServiceAPI(*configPath)
			if err != nil {
				return err
			}
			defer closer()

			events, err := api.Trace(args[0], limit)
			if err != nil {
				return err
			}

			if len(events) == 0 {
				cmd.Printf("no events found for trace %s\n", args[0])
				return nil
			}

			printEvents(cmd, events)
			return nil
		},
	}

	showCmd.Flags().IntVar(&limit, "limit", 200, "Maximum number of events for this trace")
	cmd.AddCommand(showCmd)
	return cmd
}
