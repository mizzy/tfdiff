package main

import (
	"bytes"
	"fmt"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-billy/v5/util"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"
	"io/ioutil"
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
			baseBranch, err := c.PersistentFlags().GetString("base")
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}

			err = diff(baseBranch)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		},
	}

	rootCmd.PersistentFlags().StringP("base", "b", "", "base branch")

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func diff(baseBranch string) error {
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

	p, err := exec.Command("sh", "-c", "git rev-parse --show-prefix").Output()
	if err != nil {
		return err
	}
	path := strings.TrimSpace(string(p))

	// Get resources on the base branch
	content, err := getContent(baseBranch, path)
	if err != nil {
		return err
	}
	baseResources := parse(content)

	// Get resources on the target branch
	content, err = getContent("", path)
	if err != nil {
		return err
	}
	targetResources := parse(content)

	var differentResources []string

	for name, _ := range baseResources {
		if _, ok := targetResources[name]; !ok {
			differentResources = append(differentResources, name)
			continue
		}

		if !reflect.DeepEqual(baseResources[name], targetResources[name]) {
			differentResources = append(differentResources, name)
		}
	}

	for name, _ := range targetResources {
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

func getContent(baseBranch, path string) ([]byte, error) {
	var content []byte

	if baseBranch == "" {
		files, err := filepath.Glob("*.tf")
		if err != nil {
			return content, nil
		}

		var buf bytes.Buffer
		for _, f := range files {
			c, err := ioutil.ReadFile(f)
			if err != nil {
				return content, err
			}
			buf.Write(c)
		}

		return buf.Bytes(), nil
	} else {
		r, err := exec.Command("sh", "-c", "git rev-parse --show-toplevel").Output()
		if err != nil {
			return content, err
		}
		root := strings.TrimSpace(string(r))

		storer := memory.NewStorage()
		fs := memfs.New()

		repo, err := git.Clone(storer, fs, &git.CloneOptions{
			URL: root,
		})
		if err != nil {
			return content, err
		}

		w, err := repo.Worktree()
		if err != nil {
			return content, err
		}

		err = w.Checkout(&git.CheckoutOptions{Branch: plumbing.NewBranchReferenceName(baseBranch)})

		files, err := util.Glob(fs, fmt.Sprintf("%s*.tf", path))
		if err != nil {
			return content, err
		}

		var buf bytes.Buffer
		for _, f := range files {
			c, err := util.ReadFile(fs, f)
			if err != nil {
				return content, err
			}
			buf.Write(c)
		}

		return buf.Bytes(), nil
	}

	return content, nil
}

func parse(content []byte) map[string]*Resource {
	resources := make(map[string]*Resource)
	parser := hclparse.NewParser()
	file, parseDiags := parser.ParseHCL(content, "")
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
