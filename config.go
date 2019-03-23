package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

const configFile string = "qbd.conf"

type permissions struct {
	Mode uint32 `yaml:"mode"`
	UID  int    `yaml:"uid"`
	GID  int    `yaml:"gid"`
}

type polling struct {
	Timeout uint `yaml:"timeout"`
	Delay   uint `yaml:"delay"`
}

type workers struct {
	Unpack uint `yaml:"unpack"`
	Check  uint `yaml:"check"`
}

type categories struct {
	Default     string `yaml:"default"`
	Error       string `yaml:"error"`
	NoArchive   string `yaml:"no_archive"`
	UnpackStart string `yaml:"unpack_start"`
	UnpackBusy  string `yaml:"unpack_busy"`
	UnpackDone  string `yaml:"unpack_done"`
}

type config struct {
	Server      string       `yaml:"server"`
	Port        uint16       `yaml:"port"`
	Username    string       `yaml:"username"`
	Password    string       `yaml:"password"`
	DestPath    string       `yaml:"destpath"`
	LogPath     string       `yaml:"logpath,omitempty"`
	TempPath    string       `yaml:"temppath,omitempty"`
	Permissions *permissions `yaml:"permissions,omitempty"`
	Polling     polling      `yaml:"polling"`
	Workers     workers      `yaml:"workers"`
	Categories  categories   `yaml:"categories"`
	path        string
}

func newConfig() *config {
	return &config{
		Server: "127.0.0.1",
		Port:   80,
		Polling: polling{
			Timeout: 5,
			Delay:   10,
		},
		Workers: workers{
			Unpack: 1,
			Check:  1,
		},
		Categories: categories{
			Default:     "Completed",
			Error:       "Error",
			NoArchive:   "NoArchive",
			UnpackStart: "Unpack",
			UnpackBusy:  "Unpacking",
			UnpackDone:  "Unpacked",
		},
	}
}

func (cfg *config) HasUser() bool {
	return len(cfg.Username) > 0
}

func (cfg *config) loadConfig(path string) error {
	data, err := ioutil.ReadFile(path)
	if err == nil {
		if err = yaml.Unmarshal(data, cfg); err == nil {
			cfg.path = path
		}
	}
	return err
}

func (cfg *config) writeConfig(path string, fw bool) error {
	var err error
	var fi os.FileInfo

	fi, err = os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	if fi != nil {
		if fi.IsDir() {
			path = filepath.Join(path, configFile)
		}
	}

	cb, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	fileFlags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if !fw {
		fileFlags |= os.O_EXCL
	}

	file, err := os.OpenFile(path, fileFlags, 0644)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("file already exists. Use -force to overwrite")
		}
		return err
	}

	defer file.Close()

	if _, err := file.Write(cb); err != nil {
		return err
	}

	return nil
}

func (cfg *config) Validate() error {

	// Function literal to check if a path exists and is a directory
	checkDir := func(name, path string) (err error) {
		fi, err := os.Stat(path)
		if err == nil {
			if !fi.IsDir() {
				err = fmt.Errorf("%s (%s) is not a directory", name, cfg.DestPath)
			}
		}
		return
	}

	// Check 'destpath'
	if len(cfg.DestPath) == 0 {
		return fmt.Errorf("Missing 'destpath' in %s", cfg.path)
	}

	if err := checkDir("destpath", cfg.DestPath); err != nil {
		return err
	}

	// Check 'temppath' if it's set
	if len(cfg.TempPath) > 0 {
		if err := checkDir("temppath", cfg.TempPath); err != nil {
			return err
		}
	}

	return nil
}
