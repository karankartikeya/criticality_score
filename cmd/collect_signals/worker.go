package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"

	githubstats "github.com/ossf/scorecard/v4/clients/githubrepo/stats"
	"github.com/ossf/scorecard/v4/cron/data"
	"github.com/ossf/scorecard/v4/cron/monitoring"
	"github.com/ossf/scorecard/v4/cron/worker"
	"github.com/ossf/scorecard/v4/stats"
	"go.opencensus.io/stats/view"
	"go.uber.org/zap"

	"github.com/ossf/criticality_score/v2/cmd/collect_signals/vcs"
	"github.com/ossf/criticality_score/v2/internal/collector"
	"github.com/ossf/criticality_score/v2/internal/scorer"
	"github.com/ossf/criticality_score/v2/internal/signalio"
)

const (
	collectionDateColumnName = "collection_date"
	commitIDColumnName       = "worker_commit_id"
)

type collectWorker struct {
	logger          *zap.Logger
	exporter        monitoring.Exporter
	c               *collector.Collector
	s               *scorer.Scorer
	scoreColumnName string
	csvBucketURL    string
}

// Process implements the worker.Worker interface.
func (w *collectWorker) Process(ctx context.Context, req *data.ScorecardBatchRequest, bucketURL string) error {
	filename := worker.ResultFilename(req)
	jobTime := req.GetJobTime().AsTime()
	jobID := jobTime.Format("20060102_150405")

	// Prepare the logger with identifiers for the shard and job.
	logger := w.logger.With(
		zap.Int32("shard_id", req.GetShardNum()),
		zap.Time("job_time", jobTime),
		zap.String("filename", filename),
	)
	logger.Info("Processing shard")

	// Prepare the output writer
	extras := []string{}
	if w.s != nil {
		extras = append(extras, w.scoreColumnName)
	}
	extras = append(extras, collectionDateColumnName)
	if commitID := vcs.CommitID(); commitID != vcs.MissingCommitID {
		extras = append(extras, commitIDColumnName)
	}

	var jsonOutput bytes.Buffer
	jsonOut := signalio.JSONWriter(&jsonOutput)

	var csvOutput bytes.Buffer
	csvOut := signalio.CSVWriter(&csvOutput, w.c.EmptySets(), extras...)

	// Iterate through the repos in this shard.
	for _, repo := range req.GetRepos() {
		rawURL := repo.GetUrl()
		if rawURL == "" {
			logger.Warn("Skipping empty repo URL")
			continue
		}

		// Create a logger for this repo.
		repoLogger := logger.With(zap.String("repo", rawURL))
		repoLogger.Info("Processing repo")

		// Parse the URL to ensure it is a URL.
		u, err := url.Parse(rawURL)
		if err != nil {
			// TODO: record a metric
			repoLogger.With(zap.Error(err)).Warn("Failed to parse repo URL")
			continue
		}
		ss, err := w.c.Collect(ctx, u, jobID)
		if err != nil {
			if errors.Is(err, collector.ErrUncollectableRepo) {
				repoLogger.With(zap.Error(err)).Warn("Repo is uncollectable")
				continue
			}
			return fmt.Errorf("failed during signal collection: %w", err)
		}

		// If scoring is enabled, prepare the extra data to be output.
		extras := []signalio.Field{}
		if w.s != nil {
			f := signalio.Field{
				Key:   w.scoreColumnName,
				Value: fmt.Sprintf("%.5f", w.s.Score(ss)),
			}
			extras = append(extras, f)
		}

		// Ensure the collection date is included with each record for paritioning.
		extras = append(extras, signalio.Field{
			Key:   collectionDateColumnName,
			Value: jobTime,
		})

		// Ensure the commit ID is included with each record for helping
		// identify which Git commit is associated with this record.
		if commitID := vcs.CommitID(); commitID != vcs.MissingCommitID {
			extras = append(extras, signalio.Field{
				Key:   commitIDColumnName,
				Value: commitID,
			})
		}

		// Write the signals to storage.
		if err := jsonOut.WriteSignals(ss, extras...); err != nil {
			return fmt.Errorf("failed writing signals: %w", err)
		}
		if err := csvOut.WriteSignals(ss, extras...); err != nil {
			return fmt.Errorf("failed writing signals: %w", err)
		}
	}

	// Write to the csv bucket if it is set.
	if w.csvBucketURL != "" {
		if err := data.WriteToBlobStore(ctx, w.csvBucketURL, filename, csvOutput.Bytes()); err != nil {
			return fmt.Errorf("error writing csv to blob store: %w", err)
		}
	}

	// Write to the canonical bucket last. The presence of the file indicates
	// the job was completed. See scorecard's worker package for details.
	if err := data.WriteToBlobStore(ctx, bucketURL, filename, jsonOutput.Bytes()); err != nil {
		return fmt.Errorf("error writing json to blob store: %w", err)
	}

	logger.Info("Shard written successfully")

	return nil
}

// Close is called to clean up resources used by the worker.
func (w *collectWorker) Close() {
	w.exporter.StopMetricsExporter()
}

// PostProcess implements the worker.Worker interface.
func (w *collectWorker) PostProcess() {
	w.exporter.Flush()
}

func getScorer(logger *zap.Logger, scoringEnabled bool, scoringConfigFile string) (*scorer.Scorer, error) {
	logger.Debug("Creating scorer")

	if !scoringEnabled {
		logger.Info("Scoring: disabled")
		return nil, nil
	}
	if scoringConfigFile == "" {
		logger.Info("Scoring: using default config")
		return scorer.FromDefaultConfig(), nil
	}
	logger.With(zap.String("filename", scoringConfigFile)).Info("Scoring: using config file")

	f, err := os.Open(scoringConfigFile)
	if err != nil {
		return nil, fmt.Errorf("opening config: %w", err)
	}
	defer f.Close()

	s, err := scorer.FromConfig(scorer.NameFromFilepath(scoringConfigFile), f)
	if err != nil {
		return nil, fmt.Errorf("from config: %w", err)
	}
	return s, nil
}

func getMetricsExporter() (monitoring.Exporter, error) {
	exporter, err := monitoring.GetExporter()
	if err != nil {
		return nil, fmt.Errorf("getting monitoring exporter: %w", err)
	}
	if err := exporter.StartMetricsExporter(); err != nil {
		return nil, fmt.Errorf("starting exporter: %w", err)
	}

	if err := view.Register(
		&stats.CheckRuntime,
		&stats.CheckErrorCount,
		&stats.OutgoingHTTPRequests,
		&githubstats.GithubTokens); err != nil {
		return nil, fmt.Errorf("opencensus view register: %w", err)
	}
	return exporter, nil
}

func NewWorker(ctx context.Context, logger *zap.Logger, scoringEnabled bool, scoringConfigFile, scoringColumn, csvBucketURL string, collectOpts []collector.Option) (*collectWorker, error) {
	logger.Info("Initializing worker")

	c, err := collector.New(ctx, logger, collectOpts...)
	if err != nil {
		return nil, fmt.Errorf("collector: %w", err)
	}

	s, err := getScorer(logger, scoringEnabled, scoringConfigFile)
	if err != nil {
		return nil, fmt.Errorf("scorer: %w", err)
	}

	// If we have the scorer, and the column isn't overridden, use the scorer's
	// name.
	if s != nil && scoringColumn == "" {
		scoringColumn = s.Name()
	}

	exporter, err := getMetricsExporter()
	if err != nil {
		return nil, fmt.Errorf("metrics exporter: %w", err)
	}

	return &collectWorker{
		logger:          logger,
		c:               c,
		s:               s,
		scoreColumnName: scoringColumn,
		exporter:        exporter,
		csvBucketURL:    csvBucketURL,
	}, nil
}
