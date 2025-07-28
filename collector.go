package insights

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

var COLLECTOR_EXECUTABLE_DIR = "./collectors/"
var COLLECTOR_CONFIGURATION_DIR = "./collector.d/"
var COLLECTION_DIR = "/tmp/"
var COLLECTION_DIR_PERMISSION os.FileMode = 0o750

type CollectorConfig struct {
	Meta struct {
		Name    string `toml:"name"`
		Feature string `toml:"feature"`
	} `toml:"meta"`
	Exec struct {
		Method string `toml:"method" `
		User   string `toml:"user"`
		Group  string `toml:"group"`
	} `toml:"exec"`
	MethodArchive struct {
		ContentType string `toml:"content_type"`
	} `toml:"archive"`
	MethodCustom struct {
		Command string `toml:"command"`
	} `toml:"custom"`
}

type Collector struct {
	ID     string
	Config CollectorConfig
}

func (c *Collector) GetCommand() (string, error) {
	switch c.Config.Exec.Method {
	case "archive":
		return filepath.Abs(filepath.Join(COLLECTOR_EXECUTABLE_DIR, c.ID))
	case "custom":
		return c.Config.MethodCustom.Command, nil
	}
	return "", errors.New("unknown execution method")
}

func GetCollector(id string) (*Collector, error) {
	path := filepath.Join(COLLECTOR_CONFIGURATION_DIR, id+".toml")
	config, err := getCollectorConfigFromPath(path)
	if err != nil {
		return nil, err
	}
	collector := &Collector{ID: id, Config: *config}
	return collector, nil
}

func GetCollectors() ([]*Collector, error) {
	var collectors []*Collector
	files, err := os.ReadDir(COLLECTOR_CONFIGURATION_DIR)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		if filepath.Ext(file.Name()) == ".toml" {
			id := strings.TrimSuffix(file.Name(), ".toml")
			config, err := getCollectorConfigFromPath(filepath.Join(COLLECTOR_CONFIGURATION_DIR, file.Name()))
			if err != nil {
				slog.Warn("found invalid collector", "err", err)
				continue
			}
			collector := &Collector{ID: id, Config: *config}
			collectors = append(collectors, collector)
		}
	}
	return collectors, nil
}

func getCollectorConfigFromPath(path string) (*CollectorConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return getCollectorConfigFromString(string(data))
}

func getCollectorConfigFromString(raw string) (*CollectorConfig, error) {
	var c CollectorConfig
	_, err := toml.Decode(raw, &c)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func generateCollectionDirectory(collector *Collector) (string, error) {
	path := filepath.Join(COLLECTION_DIR, collector.ID+"-"+strconv.FormatInt(time.Now().Unix(), 10))
	if err := os.MkdirAll(path, COLLECTION_DIR_PERMISSION); err != nil {
		slog.Error("cannot create collector directory", "id", collector.ID, "err", err)
		return "", fmt.Errorf("cannot create collector directory")
	}
	slog.Debug("generated collection directory", "path", path)
	return path, nil
}

// Collect instructs the collector to dump data into a temporary directory created inside COLLECTIONS_DIR.
//
// Returns path to the temporary directory, where the data has been dumped, or an error.
func Collect(collector *Collector) (string, error) {
	argv0, err := collector.GetCommand()
	if err != nil {
		return "", err
	}
	cmd := exec.Command(argv0, "collect")
	for _, variable := range os.Environ() {
		cmd.Env = append(cmd.Env, variable)
	}
	tempdir, err := generateCollectionDirectory(collector)
	if err != nil {
		return "", err
	}
	cmd.Dir = tempdir

	var stdoutBuffer, stderrBuffer bytes.Buffer
	cmd.Stdout = &stdoutBuffer
	cmd.Stderr = &stderrBuffer

	slog.Debug("executing", "cmd", cmd)
	err = cmd.Run()
	if err != nil {
		slog.Error("could not run collector", "err", err, "stderr", stderrBuffer.String())
		return "", fmt.Errorf("could not run collector: %v", err)
	}

	return tempdir, nil
}
