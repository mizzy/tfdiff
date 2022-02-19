package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	pp "github.com/k0kubun/pp/v3"
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
	currentBranch, err := exec.Command("sh", "-c", "git rev-parse --abbrev-ref HEAD").Output()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	_, err = exec.Command("sh", "-c", "git stash").Output()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	_, err = exec.Command("sh", "-c", "git checkout main || git checkout master").Output()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	mainResources := parse()

	exec.Command("sh", "-c", fmt.Sprintf("git checkout %s", currentBranch)).Output()
	exec.Command("sh", "-c", "git stash pop").Output()

	currentResources := parse()

	pp.Println(mainResources)
	pp.Println(currentResources)
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
			if block.Type == "resource" {
				resource := decodeResource(block)
				resources[resource.Name] = resource
			}
		}
	}

	return resources
}

func decodeResource(block *hclsyntax.Block) *Resource {
	r := &Resource{Name: fmt.Sprintf("%s.%s", block.Labels[0], block.Labels[1])}

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
