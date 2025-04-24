package main

import (
	"context"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v3"
)

var (
	GitHubRunID = os.Getenv("GITHUB_RUN_ID")
	JobNumber   = os.Getenv("JOB_NUMBER")
)

func runWaitReadyTest(ctx context.Context, logger *slog.Logger, index int, test *WaitURLReady) error {
	pollInterval := 1 * time.Second // Default
	if test.Interval != 0 {
		pollInterval = test.Interval
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	testStartTime := time.Now()

	// Grab the URL. It may be interpolated, so grab the variables from the environment
	// and interpolate them into the URL.
	url := test.URL
	tmpl, err := template.New("url").Parse(url)
	if err != nil {
		return err
	}
	type tmplCtx struct {
		InstanceNumber int
	}
	tctx := tmplCtx{
		InstanceNumber: index,
	}
	var rendered strings.Builder
	if err := tmpl.Execute(&rendered, tctx); err != nil {
		return err
	}
	url = rendered.String()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			test.Results.Requests.Total++

			reqLogger := logger.With("url", url, "attempt", test.Results.Requests.Total)

			// If a limit on the number of retries is set, we should check the total
			// number of requests against that limit and fail if we've exceeded it.
			if test.Retries != nil {
				if test.Results.Requests.Total > *test.Retries {
					err := fmt.Errorf("Failed to fetch: reached max retries (%d)", *test.Retries)
					reqLogger.Error(err.Error())
					return err
				}
			}

			reqLogger.Debug("Making request")

			client := &http.Client{
				CheckRedirect: func(req *http.Request, via []*http.Request) error {
					return http.ErrUseLastResponse
				},
			}

			req, err := http.NewRequest("GET", url, nil)
			if err != nil {
				reqLogger.Error("Failed to create new request", "err", err)
				continue
			}

			resp, err := client.Do(req)
			if err != nil {
				reqLogger.Error("Failed to make request", "err", err)
				continue
			}

			if err := resp.Body.Close(); err != nil {
				reqLogger.Error("Failed to close response body", "err", err)
				continue
			}

			if resp.StatusCode == test.ExpectedStatusCode {
				test.Results.Requests.Success++
				reqLogger.Info("Successfully fetched URL", "successes", test.Results.Requests.Success)

				// Set the Time to Availability(TTA) if not already set
				if test.Results.Timings.TTA == nil {
					tta := time.Since(testStartTime)
					test.Results.Timings.TTA = &tta
					logger.Info("Time to availability", "tta", test.Results.Timings.TTA)
				}
			}

			if test.Results.Requests.Success >= 1 {
				return nil
			}
		}
	}

	return nil
}

func runPlan(ctx context.Context, logger *slog.Logger, p Plan, index int) error {
	logger.Info("Running plan")

	//
	// Do any install phases
	//
	logger.Info("Running install phase for plan")
	if p.Install.Helm != nil {

		fullNameOverride := fmt.Sprintf("fullnameOverride=app-%d",
			index,
		)

		releaseName := fmt.Sprintf("%s-%d",
			p.Install.Helm.ReleaseName,
			index,
		)

		chartLogger := logger.With(
			"chart", p.Install.Helm.Chart,
			"release-name", releaseName,
			"namespace", p.Install.Helm.Namespace,
		)

		args := []string{
			"install",
			releaseName, p.Install.Helm.Chart,
			"--namespace", p.Install.Helm.Namespace,
			"--wait", "--timeout", "15m",
			"--atomic",
			"--set",
			fullNameOverride,
		}

		for k, v := range p.Install.Helm.Sets {
			args = append(args, "--set", fmt.Sprintf("%s=%s", k, v))
		}

		for _, v := range p.Install.Helm.ValuesFiles {
			args = append(args, "--values", v)
		}

		chartLogger.Debug("Installing chart", "args", args)
		cmd := exec.Command("helm", args...)
		output, err := cmd.CombinedOutput()

		if err != nil {
			chartLogger.Error("Failed to install chart", "error", err, "output", string(output))
			return err
		}

		// If we've successfully installed the operator, we want to make sure that we will
		// uninstall it after the test is done.
		defer func() {
			chartLogger.Debug("Uninstalling chart in 3 minutes")
			time.Sleep(3 * time.Minute)
			chartLogger.Debug("Uninstalling chart")
			cmd := exec.Command(
				"helm", "uninstall",
				releaseName,
				"--namespace", p.Install.Helm.Namespace,
				"--wait", "--timeout", "15m",
			)
			output, err := cmd.CombinedOutput()
			if err != nil {
				chartLogger.Error("Failed to uninstall chart", "error", err, "output", string(output))
			} else {
				chartLogger.Info("Helm uninstall complete")
			}

		}()
	}

	//
	// Do any test phases
	//
	g := errgroup.Group{}
	for _, test := range p.Tests {
		g.Go(func() error {
			if test.WaitURLReady != nil {
				logger.Info("Running wait-url-ready test")
				err := runWaitReadyTest(ctx, logger, index, test.WaitURLReady)
				if err != nil {
					return err
				}
				logger.Info("Test wait-url-ready complete")
				return nil
			}
			return nil
		})
	}
	return g.Wait()
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Error: Missing plan file argument")
		fmt.Println("")
		fmt.Println("Usage: go run ./... <plan-file>")
		os.Exit(1)
	}
	planFile := os.Args[1]

	multiWriter := io.MultiWriter(os.Stdout)
	logger := slog.New(slog.NewTextHandler(multiWriter, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	if JobNumber == "" {
		logger.Error("ENV VAR 'JOB_NUMBER' not set")
		os.Exit(1)
	}

	//
	// Config Parsing
	//
	contents, err := os.ReadFile(planFile)
	if err != nil {
		logger.Error("Failed to read file", "err", err)
		os.Exit(1)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(contents, cfg); err != nil {
		logger.Error("Failed to unmarshal YAML", "err", err)
		os.Exit(1)
	}

	//
	// Test Execution
	//
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	g, gctx := errgroup.WithContext(ctx)

	for idx, plan := range cfg.Plans {
		planLogger := logger.With("plan", plan.Name)
		g.Go(func() error { return runPlan(gctx, planLogger, plan, idx+1) })
	}

	g.Wait()
}
