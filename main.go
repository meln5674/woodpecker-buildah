package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/viper"
)

const (
	buildahPath = "/usr/bin/buildah"
)

type options struct {
	Username         string
	Password         string
	Registry         string
	Repository       string
	Tag              string
	Context          string
	ManifestName     string
	Architectures    []string
	Transport        string
	Flags            []string
	LoginArgs        []string
	ManifestArgs     []string
	BuildArgs        []string
	PushArgs         []string
	Steps            []string
	LogLevel         string
	RegistriesConfig string

	CurrentPath string
}

func main() {

	log.Println("INFO: starting buildah plugin")
	opts, err := readEnv()
	if err != nil {
		log.Fatalln("failed to execute plugin", err)
	}
	err = execute(opts)
	if err != nil {
		log.Fatalln("failed to execute plugin", err)
	}
	log.Println("INFO: finish buildah plugin")

}
func readEnv() (*options, error) {
	viper.SetEnvPrefix("plugin")
	viper.AutomaticEnv()
	viper.SetTypeByDefaultValue(true)
	viper.BindEnv("username")
	viper.BindEnv("password")
	viper.BindEnv("registry")
	viper.BindEnv("repository")
	viper.SetDefault("tag", "latest")
	viper.BindEnv("tag")
	viper.SetDefault("context", "Dockerfile")
	viper.BindEnv("context")
	viper.BindEnv("manifestname")
	viper.SetDefault("architectures", []string{"amd64"})
	viper.BindEnv("architectures")
	viper.SetDefault("transport", "docker")
	viper.BindEnv("transport")
	viper.BindEnv("flags")
	viper.BindEnv("loginargs")
	viper.BindEnv("manifestargs")
	viper.BindEnv("buildargs")
	viper.BindEnv("pushargs")
	viper.SetDefault("steps", []string{"login", "manifest", "build", "push"})
	viper.BindEnv("steps")
	viper.SetDefault("loglevel", "info") // debug, info, warn, error
	viper.BindEnv("loglevel")
	viper.BindEnv("registriesconfig")
	var opts options
	err := viper.Unmarshal(&opts)
	if err != nil {
		return nil, err
	}
	opts.CurrentPath = os.Getenv("CI_WORKSPACE")
	return &opts, nil
}
func execute(opts *options) error {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	containersDir := filepath.Join(userHome, ".config/containers")
	err = os.MkdirAll(containersDir, 0700)
	if err != nil {
		return err
	}
	if opts.RegistriesConfig != "" {
		registriesConfPath := filepath.Join(containersDir, "registries.conf")
		err = os.WriteFile(registriesConfPath, []byte(opts.RegistriesConfig), 0600)
		if err != nil {
			return err
		}
	}
	for _, step := range opts.Steps {
		switch step {
		case "login":
			err := login(opts)
			if err != nil {
				return err

			}
		case "manifest":
			err := createManifest(opts)
			if err != nil {
				return err

			}
		case "build":
			err := buildArchs(opts)
			if err != nil {
				return err

			}
		case "push":
			err := push(opts)
			if err != nil {
				return err

			}
		}
	}
	return nil
}
func login(opts *options) error {
	if len(opts.Username) == 0 || len(opts.Password) == 0 {
		return errors.New("username and password are required")
	}
	if len(opts.Registry) == 0 {
		return errors.New("registry is required")
	}

	cmd := exec.Command(buildahPath, "login", "--username", opts.Username, "--password-stdin", opts.Registry)
	cmd.Stdin = bytes.NewBufferString(opts.Password)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("login failed: %s", err.Error())
	}
	log.Println("INFO: login success at registry", opts.Registry)
	return nil
}
func createManifest(opts *options) error {
	if len(opts.ManifestName) == 0 {
		opts.ManifestName = os.Getenv("CI_COMMIT_SHA")
	}

	args := []string{"manifest", "create", opts.ManifestName, "--log-level", opts.LogLevel}
	args = append(args, opts.Flags...)
	args = append(args, opts.ManifestArgs...)
	cmd := exec.Command(buildahPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("creating manifest failed: %s", err.Error())
	}
	log.Println("INFO: created manifest", opts.ManifestName)
	return nil
}
func buildArchs(opts *options) error {

	path := opts.CurrentPath + "/" + opts.Context
	tag := opts.Registry + "/" + opts.Repository + ":" + opts.Tag
	for _, arch := range opts.Architectures {

		log.Println("INFO: building for architecture", arch)
		start := time.Now()
		args := []string{"build", "--manifest", opts.ManifestName, "--arch", arch, "--tag", tag, "--log-level", opts.LogLevel}
		args = append(args, opts.Flags...)
		args = append(args, opts.BuildArgs...)
		if !strings.Contains(runtime.GOARCH, arch) {
			log.Println("INFO: QEMU for", arch)
			args = append(args, "-f")
		}
		args = append(args, path)
		log.Println("INFO: building with args", args)
		cmd := exec.Command(buildahPath, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil {
			return fmt.Errorf("building arch %s failed: %s", arch, err.Error())
		}
		log.Println("INFO: build successfull for architecture", arch, "in", time.Since(start).Minutes(), "minutes")
	}
	log.Println("INFO: build successfull finished for tag", tag)
	return nil
}
func push(opts *options) error {
	path := opts.Transport + "://" + opts.Registry + "/" + opts.Repository + ":" + opts.Tag
	args := []string{"manifest", "push", "--all", "--log-level", opts.LogLevel, opts.ManifestName, path}
	args = append(args, opts.Flags...)
	args = append(args, opts.PushArgs...)
	cmd := exec.Command(buildahPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("pushing image failed: %s", err.Error())
	}
	log.Println("INFO: pushed successfully to", path)
	return nil
}
