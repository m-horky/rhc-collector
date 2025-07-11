package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/MatusOllah/slogcolor"
	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
	"github.com/rodaine/table"
	"github.com/urfave/cli/v3"

	. "github.com/RedHatInsights/rhc-collector"
)

func init() {
	CONFIGURATIONS_DIR = "./collectors.d/"

	{
		// TODO Log into a file
		// Configure logging
		debug := false
		for _, arg := range os.Args {
			if arg == "--debug" {
				debug = true
				break
			}
		}

		opts := slogcolor.DefaultOptions
		opts.NoColor = !isatty.IsTerminal(os.Stderr.Fd())
		opts.LevelTags = map[slog.Level]string{
			slog.LevelDebug: color.New(color.FgYellow, color.Bold).Sprint("DEBUG"),
			slog.LevelInfo:  color.New(color.FgGreen, color.Bold).Sprint("INFO"),
			slog.LevelWarn:  color.New(color.FgHiRed, color.Bold).Sprint("WARN"),
			slog.LevelError: color.New(color.FgRed, color.Bold).Sprint("ERROR"),
		}
		opts.MsgPrefix = "> "
		if debug {
			opts.Level = slog.LevelDebug
			logger := slog.New(slogcolor.NewHandler(os.Stderr, opts))
			slog.SetDefault(logger)
		} else {
			opts.Level = slog.LevelError
			slog.SetDefault(slog.New(slogcolor.NewHandler(os.Stderr, opts)))
		}
	}

	{
		// Configure ingress
		// TODO Read rhsm.conf (or equivalent)
		if useStage := os.Getenv("RHC_ENVIRONMENT"); useStage == "stage" {
			slog.Debug("using stage Ingress")
			Ingress.URL.Host = "cert.console.stage.redhat.com:443"
		}
		_ = Ingress.SetCertAuth("/etc/pki/consumer/cert.pem", "/etc/pki/consumer/key.pem")
		slog.Debug("using certificate authentication")
	}

	{
		// Configure proxy
		// TODO Support stuff like HTTPS_PROXY, NO_PROXY
		if proxyURL := os.Getenv("HTTP_PROXY"); proxyURL != "" {
			proxy, err := url.Parse(proxyURL)
			if err != nil {
				slog.Error("could not parse proxy", "error", err.Error())
			} else {
				slog.Debug("using proxy", "url", proxy)
				Ingress.Proxy = proxy
			}
		}
	}
}

var ErrorNotImplemented = fmt.Errorf("not implemented")
var ErrorNotRegistered = fmt.Errorf("this system is not registered")

func main() {
	// TODO Bash completion for collectors and flags
	cmd := &cli.Command{
		Name:            "rhc collector",
		HideHelpCommand: true,
		Usage:           "Collect and upload data",
		UsageText:       "rhc collector COMMAND [FLAGS]",
		Commands: []*cli.Command{
			{
				Name:      "run",
				Action:    doRun,
				Usage:     "run collector",
				UsageText: "rhc collector run [FLAGS] COLLECTOR",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "keep",
						Usage: "do not delete the data",
					},
					&cli.BoolFlag{
						Name:  "no-upload",
						Usage: "do not upload data",
					},
				},
				Arguments: []cli.Argument{
					&cli.StringArgs{Name: "collector", Min: 1, Max: 1},
				},
			},
			{
				Name:      "info",
				Action:    doInfo,
				Usage:     "display collector information",
				UsageText: "rhc collector info [FLAGS] COLLECTOR",
				Arguments: []cli.Argument{
					&cli.StringArgs{Name: "collector", Min: 1, Max: 1},
				},
			},
			{
				Name:      "list",
				Action:    doList,
				Usage:     "list collectors",
				UsageText: "rhc collector list [FLAGS]",
			},
			{
				Name:      "timers",
				Action:    doPs,
				Usage:     "list collector timers",
				UsageText: "rhc collector timers [FLAGS]",
			},
			{
				Name:      "enable",
				Action:    doEnable,
				Usage:     "enable collector timer",
				UsageText: "rhc collector enable [FLAGS] COLLECTOR",
				Arguments: []cli.Argument{
					&cli.StringArgs{Name: "collector", Min: 1, Max: 1},
				},
			},
			{
				Name:      "disable",
				Action:    doDisable,
				Usage:     "disable collector timer",
				UsageText: "rhc collector disable [FLAGS] COLLECTOR",
				Arguments: []cli.Argument{
					&cli.StringArgs{Name: "collector", Min: 1, Max: 1},
				},
			},
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "format",
				Usage: "output format (options: 'json')",
				Validator: func(s string) error {
					switch s {
					case "":
						return nil
					case "json":
						return nil
					default:
						return fmt.Errorf("invalid format: %s", s)
					}
				},
			},
			&cli.BoolFlag{
				Name:  "debug",
				Usage: "enable debug logging",
			},
		},
	}

	slog.Info("initiated")
	slog.Info("", slog.String("args", strings.Join(os.Args, " ")))
	if err := cmd.Run(context.Background(), os.Args); err != nil {
		fmt.Printf("Fatal error: %s\n", err)
		os.Exit(1)
	}
	slog.Info("done")
}

// CollectorInfoDTO is public API for the 'info' command.
type CollectorInfoDTO struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Feature        string `json:"feature"`
	Command        string `json:"command"`
	ContentType    string `json:"content_type"`
	UID            uint   `json:"uid"`
	GID            uint   `json:"gid"`
	DefinitionPath string `json:"path"`
	Frequency      uint   `json:"frequency"`
	Timeout        uint   `json:"timeout"`
	LastStarted    uint   `json:"last_started"`
	LastFinished   uint   `json:"last_finished"`
}

func NewCollectorInfoDTO(collector *Collector) (CollectorInfoDTO, error) {
	dto := CollectorInfoDTO{}
	dto.ID = collector.Meta.ID
	dto.Name = collector.Meta.Name
	dto.Feature = collector.Meta.Feature
	dto.Frequency = collector.Meta.Frequency
	dto.Command = collector.Exec.Command
	dto.ContentType = collector.Exec.ContentType
	dto.UID = collector.Exec.UID
	dto.GID = collector.Exec.GID
	dto.DefinitionPath = collector.Generated.Path
	dto.Timeout = collector.Exec.Timeout
	dto.DefinitionPath = collector.Generated.Path
	dto.Timeout = collector.Exec.Timeout
	{
		lastStarted, err := getLastStarted(collector)
		if err == nil {
			dto.LastStarted = uint(lastStarted.Unix())
		}
	}
	{
		lastFinished, err := getLastFinished(collector)
		if err == nil {
			dto.LastFinished = uint(lastFinished.Unix())
		}
	}

	return dto, nil
}

func doInfo(ctx context.Context, cmd *cli.Command) error {
	collector, err := GetCollector(cmd.StringArgs("collector")[0])
	if err != nil {
		slog.Error("could not find collector", "error", err)
		return err
	}

	dto, err := NewCollectorInfoDTO(collector)
	if err != nil {
		slog.Error("could not parse collector", "error", err)
	}

	switch cmd.Value("format") {
	case "json":
		return printInfoJSON(&dto)
	default:
		return printInfoHuman(&dto)
	}
}

func printInfoJSON(dto *CollectorInfoDTO) error {
	output, err := json.Marshal(dto)
	if err != nil {
		slog.Error("could not format collector info", "error", err)
		return err
	}
	fmt.Println(string(output))
	return nil
}

func printInfoHuman(dto *CollectorInfoDTO) error {
	return ErrorNotImplemented
}

func doList(ctx context.Context, cmd *cli.Command) error {
	collectors, err := GetCollectors()
	if err != nil {
		slog.Error("could not find collectors", "error", err)
		return err
	}

	dtos := make([]*CollectorInfoDTO, len(collectors))
	for i, collector := range collectors {
		dto, err := NewCollectorInfoDTO(collector)
		if err != nil {
			slog.Error("could not parse collector", "error", err)
			continue
		}
		dtos[i] = &dto
	}

	switch cmd.Value("format") {
	case "json":
		return printListJSON(dtos)
	default:
		return printListHuman(dtos)
	}
}

func printListJSON(dtos []*CollectorInfoDTO) error {
	output, err := json.Marshal(dtos)
	if err != nil {
		slog.Error("could not format collector list", "error", err)
		return err
	}
	fmt.Println(string(output))
	return nil
}

func printListHuman(dtos []*CollectorInfoDTO) error {
	// TODO Support templating like podman does?
	tbl := table.New("ID", "NAME")
	for _, collector := range dtos {
		tbl.AddRow(collector.ID, collector.Name)
	}
	tbl.Print()

	return nil
}

type CollectorRunDTO struct {
	Collector       CollectorInfoDTO `json:"collector"`
	CollectDuration float64          `json:"collect-duration"`
	UploadDuration  float64          `json:"upload-duration,omitempty"`
	Keep            bool             `json:"archive-kept"`
	KeepPath        string           `json:"archive-path,omitempty"`
	Upload          bool             `json:"archive-uploaded"`
}

func doRun(ctx context.Context, cmd *cli.Command) error {
	if err := doErrorOnMissingIdentity(); err != nil {
		slog.Debug("aborting 'run' command", slog.Any("error", err))
		return err
	}

	collector, err := GetCollector(cmd.StringArgs("collector")[0])
	if err != nil {
		slog.Error("could not find collector", "error", err)
		return err
	}
	collectorInfo, err := NewCollectorInfoDTO(collector)
	if err != nil {
		slog.Error("could not parse collector", "error", err)
		return err
	}

	if err = setLastStarted(collector); err != nil {
		slog.Error("cannot update collection timestamp", "id", collector.Meta.ID, "err", err)
	}

	keep := cmd.Bool("keep") || cmd.Bool("no-upload")
	upload := !cmd.Bool("no-upload")

	// TODO Progress messages
	collectStart := time.Now()
	tempdir, err := Collect(collector)
	collectDelta := time.Since(collectStart).Seconds()
	slog.Debug("execution finished", "collector", collector.Meta.ID, "time", collectDelta)

	defer func() {
		if keep {
			slog.Debug("keeping temporary directory", "path", tempdir)
			return
		}
		err = os.RemoveAll(tempdir)
		if err == nil {
			slog.Debug("wiped temporary directory", "path", tempdir)
		} else {
			slog.Warn("didn't wipe temporary directory", "path", tempdir, "err", err)
		}
	}()
	if err != nil {
		return err
	}

	uploadDelta := 0.0
	if upload {
		archive, err := Compress(tempdir)
		if err != nil {
			return err
		}
		defer func() {
			err = os.Remove(archive)
			if err == nil {
				slog.Debug("wiped archive", "path", archive)
			} else {
				slog.Warn("did not wipe archive", "path", archive, "err", err)
			}
		}()

		uploadSince := time.Now()
		err = Upload(archive, collector.Exec.ContentType)
		uploadDelta = time.Since(uploadSince).Seconds()
		if err != nil {
			return err
		}
	}

	dto := &CollectorRunDTO{
		Collector:       collectorInfo,
		CollectDuration: collectDelta,
		UploadDuration:  uploadDelta,
		Keep:            keep,
		Upload:          upload,
	}
	if keep {
		dto.KeepPath = tempdir
	}

	if err = setLastFinished(collector); err != nil {
		slog.Error("cannot update collection timestamp", "id", collector.Meta.ID, "err", err)
	}

	switch cmd.Value("format") {
	case "json":
		return printRunJSON(dto)
	default:
		return printRunHuman(dto)
	}
}

func printRunJSON(dto *CollectorRunDTO) error {
	output, err := json.Marshal(dto)
	if err != nil {
		return err
	}
	fmt.Println(string(output))
	return nil
}

func printRunHuman(dto *CollectorRunDTO) error {
	fmt.Printf("Finished running collection for %s.\n", dto.Collector.Name)

	fmt.Printf("Collection took %f s", dto.CollectDuration)
	if dto.Keep {
		fmt.Printf(" and has been kept in %s.\n", dto.KeepPath)
	} else {
		fmt.Print(".\n")
	}

	if dto.Upload {
		fmt.Printf("Uploading took %f s.\n", dto.UploadDuration)
	} else {
		fmt.Print("Data have not been uploaded.\n")
	}
	return nil
}

func doPs(ctx context.Context, cmd *cli.Command) error {
	switch cmd.Value("format") {
	case "json":
		return ErrorNotImplemented
	default:
		return doPsHuman(ctx, cmd)
	}
}

func doPsHuman(ctx context.Context, cmd *cli.Command) error {
	collectors, err := GetCollectors()
	if err != nil {
		return err
	}

	tbl := table.New("ID", "LAST", "NEXT")
	for _, collector := range collectors {
		var last string
		lastTimestamp, err := getLastStarted(collector)
		if err != nil {
			last = "-"
		} else {
			// TODO Show as relative: 3h 47m
			last = lastTimestamp.Format(time.RFC3339)
		}
		tbl.AddRow(collector.Meta.ID, last, "")
	}
	tbl.Print()

	fmt.Println()
	fmt.Println("Hint: Run 'rhc collector info COLLECTOR' to show more details.")

	// TODO If we are not root, pass --user
	return nil
}

func doEnable(ctx context.Context, cmd *cli.Command) error {
	// TODO If we are not root, pass --user
	return ErrorNotImplemented
}

func doDisable(ctx context.Context, cmd *cli.Command) error {
	// TODO If we are not root, pass --user
	return ErrorNotImplemented
}

func doErrorOnMissingIdentity() error {
	_, err := os.Stat("/etc/pki/consumer/cert.pem")
	if err != nil {
		fmt.Println("This system is not registered, run 'rhc connect' before continuing.")
		return ErrorNotRegistered
	}
	return nil
}

func setLastStarted(c *Collector) error {
	now := strconv.FormatInt(time.Now().Unix(), 10)
	path := filepath.Join(CACHE_DIR, c.Meta.ID+".last-started")
	err := os.WriteFile(path, []byte(now), 0644)
	if err == nil {
		slog.Debug("cached last started timestamp", "path", path)
	} else {
		slog.Error("could not cache timestamp", "path", path, "error", err)
	}
	return err
}

func getLastStarted(c *Collector) (time.Time, error) {
	file := filepath.Join(CACHE_DIR, c.Meta.ID+".last-started")
	raw, err := os.ReadFile(file)
	if err != nil {
		slog.Debug("cannot parse timestamp", "file", file, "err", err)
		return time.Time{}, err
	}
	i, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil {
		slog.Warn("cannot parse timestamp", "file", file, "err", err)
		return time.Time{}, err
	}
	return time.Unix(i, 0), nil
}

func setLastFinished(c *Collector) error {
	now := strconv.FormatInt(time.Now().Unix(), 10)
	path := filepath.Join(CACHE_DIR, c.Meta.ID+".last-finished")
	err := os.WriteFile(path, []byte(now), 0644)
	if err == nil {
		slog.Debug("cached last finished timestamp", "path", path)
	} else {
		slog.Error("could not cache timestamp", "path", path, "error", err)
	}
	return err
}

func getLastFinished(c *Collector) (time.Time, error) {
	file := filepath.Join(CACHE_DIR, c.Meta.ID+".last-finished")
	raw, err := os.ReadFile(file)
	if err != nil {
		slog.Debug("cannot parse timestamp", "file", file, "err", err)
		return time.Time{}, err
	}
	i, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil {
		slog.Warn("cannot parse timestamp", "file", file, "err", err)
		return time.Time{}, err
	}
	return time.Unix(i, 0), nil
}
