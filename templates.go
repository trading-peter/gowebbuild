package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"text/template"

	"github.com/Iilun/survey/v2"
	"github.com/kataras/golog"
	"github.com/urfave/cli/v2"
)

//go:embed templates/tpl.gowebbuild.yaml
var sampleConfig string

//go:embed templates/docker_image.sh
var dockerImage string

//go:embed templates/Dockerfile
var dockerFile string

//go:embed templates/.air.toml
var airToml string

//go:embed templates/.air.win.toml
var airWinToml string

var qs = []*survey.Question{
	{
		Name: "tpl",
		Prompt: &survey.Select{
			Message: "Choose a template:",
			Options: []string{".gowebbuild.yaml", "docker_image.sh", "Dockerfile"},
			Default: "docker_image.sh",
		},
	},
}

func tplAction(ctx *cli.Context) error {
	cfgPath, err := filepath.Abs(ctx.String("c"))

	if err != nil {
		return err
	}

	os.Chdir(filepath.Dir(cfgPath))

	answers := struct {
		Template string `survey:"tpl"`
	}{}

	err = survey.Ask(qs, &answers)
	if err != nil {
		return err
	}

	var tpl string
	var fileName string

	switch answers.Template {
	case ".gowebbuild.yaml":
		tpl = sampleConfig
		fileName = ".gowebbuild.yaml"
	case "docker_image.sh":
		tpl = dockerImage
		fileName = "docker_image.sh"
	case "Dockerfile":
		tpl = dockerFile
		fileName = "Dockerfile"
	case "air.toml":
		tpl = airToml
		if runtime.GOOS == "windows" {
			tpl = airWinToml
		}
		fileName = ".air.toml"
	default:
		golog.Fatal("Invalid template")
	}

	if isFile(fileName) {
		fmt.Printf("File \"%s\" already exists.\n", fileName)
		return nil
	}

	outFile, err := os.Create(fileName)
	if err != nil {
		return err
	}

	defer outFile.Close()

	context := map[string]string{
		"ProjectFolderName": filepath.Base(filepath.Dir(cfgPath)),
	}

	if moduleName, err := getGoModuleName(filepath.Dir(cfgPath)); err == nil {
		context["GoModuleName"] = moduleName
	}

	err = template.Must(template.New("tpl").Parse(tpl)).Execute(outFile, context)

	if err != nil {
		return err
	}

	fmt.Printf("Created \"%s\" in project root.\n", fileName)

	return nil
}
