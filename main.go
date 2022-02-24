package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/spf13/cobra"
	"github.com/zclconf/go-cty/cty"
)

type Resource struct {
	Name       string
	Attributes map[string]cty.Value
	Blocks     map[string]Block
}

type Block struct {
	Attributes map[string]cty.Value
	Blocks     map[string]Block
}

func main() {
	rootCmd := &cobra.Command{
		Run: func(c *cobra.Command, args []string) {
			base, err := c.PersistentFlags().GetString("base")
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}

			target, err := c.PersistentFlags().GetString("target")
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}

			err = diff(base, target)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		},
	}

	rootCmd.PersistentFlags().StringP("base", "b", "", "base branch")
	rootCmd.PersistentFlags().StringP("target", "t", "", "target branch")

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func diff(baseBranch, targetBranch string) error {
	if baseBranch == "" {
		_, err := exec.Command("sh", "-c", "git branch | grep -q main").Output()
		if err == nil {
			baseBranch = "main"
		}

		_, err = exec.Command("sh", "-c", "git branch | grep -q master").Output()
		if err == nil {
			baseBranch = "master"
		}

		if baseBranch == "" {
			return fmt.Errorf("can't specify base branch")
		}
	}

	if targetBranch == "" {
		t, err := exec.Command("sh", "-c", "git rev-parse --abbrev-ref HEAD").Output()
		if err != nil {
			return err
		}
		targetBranch = strings.TrimSpace(string(t))
	}

	_, err := exec.Command("sh", "-c", "git stash").Output()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	_, err = exec.Command("sh", "-c", fmt.Sprintf("git checkout %s", baseBranch)).Output()
	if err != nil {
		return err
	}

	baseResources := parse()

	exec.Command("sh", "-c", fmt.Sprintf("git checkout %s", targetBranch)).Output()
	exec.Command("sh", "-c", "git stash pop").Output()

	headResources := parse()

	var differentResources []string

	for name, _ := range baseResources {
		if _, ok := headResources[name]; !ok {
			differentResources = append(differentResources, name)
			continue
		}

		if !reflect.DeepEqual(baseResources[name], headResources[name]) {
			differentResources = append(differentResources, name)
		}
	}

	for name, _ := range headResources {
		if _, ok := baseResources[name]; !ok {
			differentResources = append(differentResources, name)
		}
	}

	if len(differentResources) > 0 {
		for _, r := range differentResources {
			fmt.Printf("-target %s ", r)
		}
	} else {
		fmt.Print("-refresh=false")
	}

	return nil
}

func parse() map[string]*Resource {
	parser := hclparse.NewParser()

	files, err := filepath.Glob("*.tf")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	resources := make(map[string]*Resource)

	for _, f := range files {
		file, parseDiags := parser.ParseHCLFile(f)
		if parseDiags.HasErrors() {
			fmt.Println(parseDiags.Error())
			os.Exit(1)
		}

		for _, block := range reflect.ValueOf(file.Body).Elem().Interface().(hclsyntax.Body).Blocks {
			if block.Type == "resource" || block.Type == "module" {
				resource := decodeResource(block)
				resources[resource.Name] = resource
			}
		}
	}

	return resources
}

func decodeResource(block *hclsyntax.Block) *Resource {
	r := &Resource{}

	if block.Type == "resource" {
		r.Name = fmt.Sprintf("%s.%s", block.Labels[0], block.Labels[1])
	} else if block.Type == "module" {
		r.Name = fmt.Sprintf("module.%s", block.Labels[0])
	}

	if len(block.Body.Attributes) > 0 {
		r.Attributes = decodeAttributes(block.Body.Attributes)
	}

	if len(block.Body.Blocks) > 0 {
		r.Blocks = decodeBlocks(block.Body.Blocks)
	}

	return r
}

func decodeAttributes(attributes hclsyntax.Attributes) map[string]cty.Value {
	a := make(map[string]cty.Value)

	for _, attr := range attributes {
		v, _ := attr.Expr.Value(&hcl.EvalContext{})
		a[attr.Name] = v
	}

	return a
}

func decodeBlocks(blocks hclsyntax.Blocks) map[string]Block {
	block := make(map[string]Block)

	for _, b := range blocks {
		n := Block{}
		if len(b.Body.Attributes) > 0 {
			n.Attributes = decodeAttributes(b.Body.Attributes)
		}

		if len(b.Body.Blocks) > 0 {
			n.Blocks = decodeBlocks(b.Body.Blocks)
		}

		block[b.Type] = n
	}

	return block
}
