package run

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

type RunConfig struct {
	Enabled          bool     `toml:"enabled"`
	CompatibilityKey string   `toml:"compatibilityKey"`
	Name             string   `toml:"name"`
	Description      string   `toml:"description"`
	Source           string   `toml:"source"`
	Includes         []string `toml:"Includes"`
	Excludes         []string `toml:"excludes"`
	Destination      `toml:"destination"`
	Rotations        `toml:"rotations"`
	lastRun          time.Time
}

type Destination struct {
	Path          string `toml:"path"`
	RemoteHost    string `toml:"remoteHost"`
	Username      string `toml:"username"`
	Port          string `toml:"port"`
	PrivateKeyUrl string `toml:"privateKeyUrl"`
}

type Rotations struct {
	Frequency int `toml:"frequency"`
	Delay     int `toml:"delay"`
	Initial   int `toml:"initial"`
	Daily     int `toml:"daily"`
	Monthly   int `toml:"monthly"`
	Yearly    int `toml:"yearly"`
}

func (rc *RunConfig) WriteRunConfig() (err error) {
	err = rc.generateCompatibilityKey()
	if err != nil {
		return
	}
	buf := new(bytes.Buffer)
	err = toml.NewEncoder(buf).Encode(rc)
	if err != nil {
		return
	}
	newRunConfigFile := filepath.Join(config.RunConfigDir, rc.Name+".toml")
	err = ioutil.WriteFile(newRunConfigFile, buf.Bytes(), 0644)
	if err != nil {
		err = fmt.Errorf("Error writing run config to file %s: %s", newRunConfigFile, err.Error())
		return
	}
	return
}

func (rc *RunConfig) Enable() error {
	rc.Enabled = true
	err := rc.WriteRunConfig()
	if err != nil {
		err = fmt.Errorf("Error enabling run config: %s", err.Error())
	}
	return err
}

func (rc *RunConfig) Disable() error {
	rc.Enabled = false
	err := rc.WriteRunConfig()
	if err != nil {
		err = fmt.Errorf("Error disabling run config: %s", err.Error())
	}
	return err
}

func (rc *RunConfig) IsEnabled() bool {
	return rc.Enabled
}

func (r *RunConfig) generateCompatibilityKey() error {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return fmt.Errorf("Error generating compatibility key: %s", err.Error())
	}
	compatibilityKey := fmt.Sprintf("%x", b)
	r.CompatibilityKey = compatibilityKey
	return nil
}

func GetRunConfig(name string) (rc RunConfig, err error) {
	rcs, err := getRunConfigs(true, true)
	if err != nil {
		return
	}
	for _, c := range rcs {
		if c.Name == name {
			rc = c
			break
		}
	}
	if rc.Name != name {
		err = fmt.Errorf("Error finding run config with the name %s", name)
	}
	return
}

func GetRunConfigs() (rcs []RunConfig, err error) {
	return getRunConfigs(true, true)
}

func GetEnabledConfigs() (rcs []RunConfig, err error) {
	return getRunConfigs(true, false)
}

func GetDisabledConfigs() (rcs []RunConfig, err error) {
	return getRunConfigs(false, true)
}

func EnableRunConfig(name string) error {
	rc, err := GetRunConfig(name)
	if err != nil {
		return err
	}
	err = rc.Enable()
	if err != nil {
		err = fmt.Errorf("Error enabling run config: %s", err.Error())
	}
	return err
}

func DisableRunConfig(name string) error {
	rc, err := GetRunConfig(name)
	if err != nil {
		return err
	}
	err = rc.Disable()
	if err != nil {
		err = fmt.Errorf("Error disabling run config: %s", err.Error())
	}
	return err
}

func decodeRunConfig(path string) (rc RunConfig, err error) {
	if _, e := os.Stat(path); os.IsNotExist(e) {
		err = fmt.Errorf("run config file %s does not exist", path)
		return
	}
	_, err = toml.DecodeFile(path, &rc)
	if err != nil {
		err = fmt.Errorf("Error decoding run config file %s: %s", path, err.Error())
		return
	}
	return
}

func getRunConfigs(enabled bool, disabled bool) (rcs []RunConfig, err error) {
	runConfigChan := make(chan RunConfig, 8)
	errChan := make(chan error, 1)
	go func() {
		err := filepath.Walk(config.RunConfigDir, func(path string, f os.FileInfo, err error) error {
			if f.IsDir() {
				if filepath.Clean(path) != filepath.Clean(config.RunConfigDir) {
					return filepath.SkipDir
				}
				return nil
			}
			ok, err := filepath.Match("*.toml", f.Name())
			if !ok {
				return nil
			}
			ok, err = filepath.Match(".*.toml", f.Name())
			if ok {
				return nil
			}
			c, e := decodeRunConfig(path)
			switch {
			case enabled && c.Enabled:
				runConfigChan <- c
			case disabled && !c.Enabled:
				runConfigChan <- c
			case e != nil:
				errChan <- e
			}
			return nil
		})
		if err != nil {
			errChan <- err
		}
		close(runConfigChan)
	}()
CollectRCs:
	for {
		select {
		case e := <-errChan:
			err = fmt.Errorf("Error parsing run config directory: %s", e.Error())
			break CollectRCs
		case rc, ok := <-runConfigChan:
			if !ok {
				break CollectRCs
			}
			rcs = append(rcs, rc)
		}
	}
	return
}