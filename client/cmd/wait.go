package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jimeng-relay/client/internal/jimeng"
	"github.com/spf13/cobra"
)

type waitFlagValues struct {
	taskID      string
	interval    string
	waitTimeout string
}

var waitFlags waitFlagValues

var waitCmd = &cobra.Command{
	Use:   "wait",
	Short: "Wait for task completion",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, formatter, err := newClientAndFormatter(cmd)
		if err != nil {
			return err
		}

		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}

		var interval time.Duration
		if raw := strings.TrimSpace(waitFlags.interval); raw != "" {
			d, err := time.ParseDuration(raw)
			if err != nil {
				return fmt.Errorf("invalid --interval: %w", err)
			}
			interval = d
		}
		var timeout time.Duration
		if raw := strings.TrimSpace(waitFlags.waitTimeout); raw != "" {
			d, err := time.ParseDuration(raw)
			if err != nil {
				return fmt.Errorf("invalid --wait-timeout: %w", err)
			}
			timeout = d
		}

		res, err := client.Wait(ctx, waitFlags.taskID, jimeng.WaitOptions{Interval: interval, Timeout: timeout})
		if err != nil {
			return err
		}

		resp := &jimeng.GetResultResponse{Status: res.FinalStatus, ImageURLs: res.ImageURLs}
		out, err := formatter.FormatGetResultResponse(resp)
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), out)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(waitCmd)

	waitCmd.Flags().StringVar(&waitFlags.taskID, "task-id", "", "Task ID")
	waitCmd.Flags().StringVar(&waitFlags.interval, "interval", "", "Poll interval duration, e.g. 2s")
	waitCmd.Flags().StringVar(&waitFlags.waitTimeout, "wait-timeout", "", "Max wait duration, e.g. 60s, 5m")
	if err := waitCmd.MarkFlagRequired("task-id"); err != nil {
		panic(err)
	}
}
