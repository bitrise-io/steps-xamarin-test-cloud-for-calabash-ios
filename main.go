package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/command/rubycommand"
	"github.com/bitrise-io/go-utils/fileutil"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/pathutil"
	shellquote "github.com/kballard/go-shellquote"
)

// ConfigsModel ...
type ConfigsModel struct {
	WorkDir     string
	GemFilePath string

	IpaPath  string
	DsymPath string

	User          string
	APIKey        string
	Devices       string
	IsAsync       string
	Series        string
	CustomOptions string
}

func createConfigsModelFromEnvs() ConfigsModel {
	return ConfigsModel{
		WorkDir:     os.Getenv("work_dir"),
		GemFilePath: os.Getenv("gem_file_path"),

		IpaPath:  os.Getenv("ipa_path"),
		DsymPath: os.Getenv("dsym_path"),

		User:          os.Getenv("xamarin_user"),
		APIKey:        os.Getenv("test_cloud_api_key"),
		Devices:       os.Getenv("test_cloud_devices"),
		IsAsync:       os.Getenv("test_cloud_is_async"),
		Series:        os.Getenv("test_cloud_series"),
		CustomOptions: os.Getenv("other_parameters"),
	}
}

func (configs ConfigsModel) print() {
	log.Infof("Configs:")

	log.Printf("- WorkDir: %s", configs.WorkDir)
	log.Printf("- GemFilePath: %s", configs.GemFilePath)

	log.Printf("- IpaPath: %s", configs.IpaPath)
	log.Printf("- DsymPath: %s", configs.DsymPath)

	log.Printf("- User: %s", configs.User)
	log.Printf("- APIKey: %s", configs.APIKey)
	log.Printf("- Devices: %s", configs.Devices)
	log.Printf("- IsAsync: %s", configs.IsAsync)
	log.Printf("- Series: %s", configs.Series)
	log.Printf("- CustomOptions: %s", configs.CustomOptions)
}

func (configs ConfigsModel) validate() error {
	if configs.WorkDir == "" {
		return errors.New("no WorkDir parameter specified")
	}
	if exist, err := pathutil.IsDirExists(configs.WorkDir); err != nil {
		return fmt.Errorf("failed to check if WorkDir exist, error: %s", err)
	} else if !exist {
		return fmt.Errorf("WorkDir directory not exists at: %s", configs.WorkDir)
	}

	if configs.IpaPath != "" {
		if exist, err := pathutil.IsPathExists(configs.IpaPath); err != nil {
			return fmt.Errorf("failed to check if IpaPath exist, error: %s", err)
		} else if !exist {
			return fmt.Errorf("IpaPath directory not exists at: %s", configs.IpaPath)
		}
	} else {
		return errors.New("no IpaPath parameter specified")
	}

	if configs.DsymPath != "" {
		if exist, err := pathutil.IsDirExists(configs.DsymPath); err != nil {
			return fmt.Errorf("failed to check if DsymPath exist, error: %s", err)
		} else if !exist {
			return fmt.Errorf("DsymPath directory not exists at: %s", configs.DsymPath)
		}
	}

	if configs.User == "" {
		return errors.New("no User parameter specified")
	}
	if configs.APIKey == "" {
		return errors.New("no APIKey parameter specified")
	}
	if configs.Devices == "" {
		return errors.New("no Devices parameter specified")
	}
	if configs.IsAsync == "" {
		return errors.New("no IsAsync parameter specified")
	}
	if configs.Series == "" {
		return errors.New("no Series parameter specified")
	}

	return nil
}

// JSONResultModel ...
type JSONResultModel struct {
	Log           []string `json:"Log"`
	ErrorMessages []string `json:"ErrorMessages"`
	TestRunID     string   `json:"TestRunId"`
	LaunchURL     string   `json:"LaunchUrl"`
}

func exportEnvironmentWithEnvman(keyStr, valueStr string) error {
	cmd := command.New("envman", "add", "--key", keyStr)
	cmd.SetStdin(strings.NewReader(valueStr))
	return cmd.Run()
}

func testResultLogContent(pth string) (string, error) {
	if exist, err := pathutil.IsPathExists(pth); err != nil {
		return "", fmt.Errorf("Failed to check if path (%s) exist, error: %s", pth, err)
	} else if !exist {
		return "", fmt.Errorf("test result not exist at: %s", pth)
	}

	content, err := fileutil.ReadStringFromFile(pth)
	if err != nil {
		return "", fmt.Errorf("Failed to read file (%s), error: %s", pth, err)
	}

	return content, nil
}

func gemVersionFromGemfileLockContent(gemName, gemfileLockContent string) string {
	relevantLines := []string{}
	lines := strings.Split(gemfileLockContent, "\n")

	specsStart := false
	for _, line := range lines {
		if strings.Contains(line, "specs:") {
			specsStart = true
		}

		trimmed := strings.Trim(line, " ")
		if trimmed == "" {
			break
		}

		if specsStart {
			relevantLines = append(relevantLines, line)
		}
	}

	pattern := fmt.Sprintf(`%s \((.+)\)`, gemName)
	exp := regexp.MustCompile(pattern)
	for _, line := range relevantLines {
		match := exp.FindStringSubmatch(line)
		if match != nil && len(match) == 2 {
			return match[1]
		}
	}

	return ""
}

func gemVersionFromGemfileLock(gemName, gemfileLockPth string) (string, error) {
	content, err := fileutil.ReadStringFromFile(gemfileLockPth)
	if err != nil {
		return "", err
	}
	return gemVersionFromGemfileLockContent(gemName, content), nil
}

func registerFail(format string, v ...interface{}) {
	log.Errorf(format, v...)

	if err := exportEnvironmentWithEnvman("BITRISE_XAMARIN_TEST_RESULT", "failed"); err != nil {
		log.Warnf("Failed to export environment: %s, error: %s", "BITRISE_XAMARIN_TEST_RESULT", err)
	}

	os.Exit(1)
}

func main() {
	configs := createConfigsModelFromEnvs()

	fmt.Println()
	configs.print()

	if err := configs.validate(); err != nil {
		registerFail("Issue with input: %s", err)
	}

	//
	// Determining cucumber & test-cloud version
	fmt.Println()
	log.Infof("Determining cucumber & test-cloud version...")

	workDir, err := pathutil.AbsPath(configs.WorkDir)
	if err != nil {
		registerFail("Failed to expand WorkDir (%s), error: %s", configs.WorkDir, err)
	}

	gemFilePath := ""
	if configs.GemFilePath != "" {
		gemFilePath, err = pathutil.AbsPath(configs.GemFilePath)
		if err != nil {
			registerFail("Failed to expand GemFilePath (%s), error: %s", configs.GemFilePath, err)
		}
	}

	useBundlerForCalabash := false
	useBundlerForTestCloud := false

	if gemFilePath != "" {
		if exist, err := pathutil.IsPathExists(gemFilePath); err != nil {
			registerFail("Failed to check if Gemfile exists at (%s) exist, error: %s", gemFilePath, err)
		} else if exist {
			log.Printf("Gemfile exists at: %s", gemFilePath)

			gemfileDir := filepath.Dir(gemFilePath)
			gemfileLockPth := filepath.Join(gemfileDir, "Gemfile.lock")

			if exist, err := pathutil.IsPathExists(gemfileLockPth); err != nil {
				registerFail("Failed to check if Gemfile.lock exists at (%s), error: %s", gemfileLockPth, err)
			} else if exist {
				log.Printf("Gemfile.lock exists at: %s", gemfileLockPth)

				{
					version, err := gemVersionFromGemfileLock("cucumber", gemfileLockPth)
					if err != nil {
						registerFail("Failed to get cucumber version from Gemfile.lock, error: %s", err)
					}

					if version != "" {
						log.Printf("cucumber version in Gemfile.lock: %s", version)
						useBundlerForCalabash = true
					}
				}

				{
					version, err := gemVersionFromGemfileLock("xamarin-test-cloud", gemfileLockPth)
					if err != nil {
						registerFail("Failed to get xamarin-test-cloud version from Gemfile.lock, error: %s", err)
					}

					if version != "" {
						log.Printf("xamarin-test-cloud version in Gemfile.lock: %s", version)
						useBundlerForTestCloud = true
					}
				}
			} else {
				log.Warnf("Gemfile.lock doest no find with cucumber gem at: %s", gemfileLockPth)
			}
		} else {
			log.Warnf("Gemfile doest no find with cucumber gem at: %s", gemFilePath)
		}
	}

	if useBundlerForCalabash {
		log.Donef("using cucumber with bundler")
	} else {
		log.Donef("using cucumber latest version")
	}

	if useBundlerForTestCloud {
		log.Donef("using xamarin-test-cloud with bundler")
	} else {
		log.Donef("using xamarin-test-cloud latest version")
	}
	// ---

	//
	// Intsalling cucumber gem
	fmt.Println()
	log.Infof("Installing cucumber gem...")

	if useBundlerForCalabash || useBundlerForTestCloud {
		bundleInstallCmd, err := rubycommand.New("bundle", "install", "--jobs", "20", "--retry", "5")
		if err != nil {
			registerFail("Failed to create command, error: %s", err)
		}

		bundleInstallCmd.AppendEnvs("BUNDLE_GEMFILE=" + gemFilePath)
		bundleInstallCmd.SetStdout(os.Stdout).SetStderr(os.Stderr)

		log.Printf("$ %s", bundleInstallCmd.PrintableCommandArgs())

		if err := bundleInstallCmd.Run(); err != nil {
			registerFail("bundle install failed, error: %s", err)
		}
	}

	if !useBundlerForCalabash {
		installCommands, err := rubycommand.GemInstall("cucumber", "")
		if err != nil {
			registerFail("Failed to create gem install commands, error: %s", err)
		}

		for _, installCommand := range installCommands {
			log.Printf("$ %s", command.PrintableCommandArgs(false, installCommand.GetCmd().Args))

			installCommand.SetStdout(os.Stdout).SetStderr(os.Stderr)

			if err := installCommand.Run(); err != nil {
				registerFail("command failed, error: %s", err)
			}
		}
	}

	if !useBundlerForTestCloud {
		installCommands, err := rubycommand.GemInstall("xamarin-test-cloud", "")
		if err != nil {
			registerFail("Failed to create gem install commands, error: %s", err)
		}

		for _, installCommand := range installCommands {
			log.Printf("$ %s", command.PrintableCommandArgs(false, installCommand.GetCmd().Args))

			installCommand.SetStdout(os.Stdout).SetStderr(os.Stderr)

			if err := installCommand.Run(); err != nil {
				registerFail("command failed, error: %s", err)
			}
		}
	}

	// ---

	//
	// Submit ipa
	fmt.Println()
	log.Infof("Submit ipa...")

	submitEnvs := []string{}
	submitArgs := []string{"test-cloud"}
	if useBundlerForTestCloud {
		submitArgs = append([]string{"bundle", "exec"}, submitArgs...)
		submitEnvs = append(submitEnvs, "BUNDLE_GEMFILE="+gemFilePath)
	}

	submitArgs = append(submitArgs, "submit", configs.IpaPath, configs.APIKey)
	submitArgs = append(submitArgs, fmt.Sprintf("--user=%s", configs.User))
	submitArgs = append(submitArgs, fmt.Sprintf("--devices=%s", configs.Devices))
	if configs.IsAsync == "yes" {
		submitArgs = append(submitArgs, "--async")
	}
	if configs.Series != "" {
		submitArgs = append(submitArgs, fmt.Sprintf("--series=%s", configs.Series))
	}

	if configs.CustomOptions != "" {
		options, err := shellquote.Split(configs.CustomOptions)
		if err != nil {
			registerFail("Failed to shell split CustomOptions (%s), error: %s", configs.CustomOptions, err)
		}

		submitArgs = append(submitArgs, options...)
	}

	submitCmd, err := rubycommand.NewFromSlice(submitArgs...)
	if err != nil {
		registerFail("Failed to create command, error: %s", err)
	}

	submitCmd.AppendEnvs(submitEnvs...)
	submitCmd.SetDir(workDir)
	submitCmd.SetStdout(os.Stdout).SetStderr(os.Stderr)

	log.Printf("$ %s", submitCmd.PrintableCommandArgs())
	fmt.Println()

	if err := submitCmd.Run(); err != nil {
		registerFail("Failed to run command, error: %s", err)
	}
}
