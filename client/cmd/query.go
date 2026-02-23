package cmd

import (
	"context"
	"fmt"

	"github.com/jimeng-relay/client/internal/jimeng"
	"github.com/spf13/cobra"
)

type queryFlagValues struct {
	taskID string
}

var queryFlags queryFlagValues

var queryCmd = &cobra.Command{
	Use:   "query",
	Short: "Query task status",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, formatter, err := newClientAndFormatter(cmd)
		if err != nil {
			return err
		}

		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}

		resp, err := client.GetResult(ctx, jimeng.GetResultRequest{TaskID: queryFlags.taskID})
		if err != nil {
			return err
		}
		out, err := formatter.FormatGetResultResponse(resp)
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), out)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(queryCmd)

	queryCmd.Flags().StringVar(&queryFlags.taskID, "task-id", "", "Task ID")
	_ = queryCmd.MarkFlagRequired("task-id")
}
