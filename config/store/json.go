package store

import (
	gojson "encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/datarhei/core/v16/config"
	v1 "github.com/datarhei/core/v16/config/v1"
	v2 "github.com/datarhei/core/v16/config/v2"
	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/io/file"
)

type jsonStore struct {
	path string

	data map[string]*config.Config

	reloadFn func()
}

// NewJSONStore will read a JSON config file from the given path. After successfully reading it in, it will be written
// back to the path. The returned error will be nil if everything went fine.
// If the path doesn't exist, a default JSON config file will be written to that path.
// The returned ConfigStore can be used to retrieve or write the config.
func NewJSON(path string, reloadFn func()) (Store, error) {
	c := &jsonStore{
		path:     path,
		data:     make(map[string]*config.Config),
		reloadFn: reloadFn,
	}

	c.data["base"] = config.New()

	if err := c.load(c.data["base"]); err != nil {
		return nil, fmt.Errorf("failed to read JSON from '%s': %w", path, err)
	}

	if err := c.store(c.data["base"]); err != nil {
		return nil, fmt.Errorf("failed to write JSON to '%s': %w", path, err)
	}

	return c, nil
}

func (c *jsonStore) Get() *config.Config {
	return c.data["base"].Clone()
}

func (c *jsonStore) Set(d *config.Config) error {
	if d.HasErrors() {
		return fmt.Errorf("configuration data has errors after validation")
	}

	data := d.Clone()

	if err := c.store(data); err != nil {
		return fmt.Errorf("failed to write JSON to '%s': %w", c.path, err)
	}

	c.data["base"] = data

	return nil
}

func (c *jsonStore) GetActive() *config.Config {
	if x, ok := c.data["merged"]; ok {
		return x.Clone()
	}

	if x, ok := c.data["base"]; ok {
		return x.Clone()
	}

	return nil
}

func (c *jsonStore) SetActive(d *config.Config) error {
	d.Validate(true)

	if d.HasErrors() {
		return fmt.Errorf("configuration data has errors after validation")
	}

	data := d.Clone()

	c.data["merged"] = data

	return nil
}

func (c *jsonStore) Reload() error {
	if c.reloadFn == nil {
		return nil
	}

	c.reloadFn()

	return nil
}

func (c *jsonStore) load(cfg *config.Config) error {
	if len(c.path) == 0 {
		return nil
	}

	if _, err := os.Stat(c.path); os.IsNotExist(err) {
		return nil
	}

	jsondata, err := os.ReadFile(c.path)
	if err != nil {
		return err
	}

	if len(jsondata) == 0 {
		return nil
	}

	data, err := migrate(jsondata)
	if err != nil {
		return err
	}

	cfg.Data = *data

	cfg.UpdatedAt = cfg.CreatedAt

	return nil
}

func (c *jsonStore) store(data *config.Config) error {
	if len(c.path) == 0 {
		return nil
	}

	jsondata, err := gojson.MarshalIndent(data, "", "    ")
	if err != nil {
		return err
	}

	dir, filename := filepath.Split(c.path)

	tmpfile, err := os.CreateTemp(dir, filename)
	if err != nil {
		return err
	}

	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write(jsondata); err != nil {
		return err
	}

	if err := tmpfile.Close(); err != nil {
		return err
	}

	if err := file.Rename(tmpfile.Name(), c.path); err != nil {
		return err
	}

	return nil
}

func migrate(jsondata []byte) (*config.Data, error) {
	data := &config.Data{}
	version := DataVersion{}

	if err := gojson.Unmarshal(jsondata, &version); err != nil {
		return nil, json.FormatError(jsondata, err)
	}

	if version.Version == 1 {
		dataV1 := &v1.New().Data

		if err := gojson.Unmarshal(jsondata, dataV1); err != nil {
			return nil, json.FormatError(jsondata, err)
		}

		dataV2, err := v2.UpgradeV1ToV2(dataV1)
		if err != nil {
			return nil, err
		}

		dataV3, err := config.UpgradeV2ToV3(dataV2)
		if err != nil {
			return nil, err
		}

		data = dataV3
	} else if version.Version == 2 {
		dataV2 := &v2.New().Data

		if err := gojson.Unmarshal(jsondata, dataV2); err != nil {
			return nil, json.FormatError(jsondata, err)
		}

		dataV3, err := config.UpgradeV2ToV3(dataV2)
		if err != nil {
			return nil, err
		}

		data = dataV3
	} else if version.Version == 3 {
		dataV3 := &config.New().Data

		if err := gojson.Unmarshal(jsondata, dataV3); err != nil {
			return nil, json.FormatError(jsondata, err)
		}

		data = dataV3
	}

	return data, nil
}
