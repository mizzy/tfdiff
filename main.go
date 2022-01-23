package main

import (
	"fmt"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/k0kubun/pp/v3"
	"github.com/zclconf/go-cty/cty"
	"os"
	"reflect"
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
	parser := hclparse.NewParser()

	file, parseDiags := parser.ParseHCLFile("s3.tf")
	if parseDiags.HasErrors() {
		fmt.Println(parseDiags.Error())
		os.Exit(1)
	}

	for _, block := range reflect.ValueOf(file.Body).Elem().Interface().(hclsyntax.Body).Blocks {
		if block.Type == "resource" {
			decodeResource(block)
		}
	}
}

func decodeResource(block *hclsyntax.Block) {
	r := &Resource{Name: fmt.Sprintf("%s.%s", block.Labels[0], block.Labels[1])}

	if len(block.Body.Attributes) > 0 {
		r.Attributes = decodeAttributes(block.Body.Attributes)
	}

	if len(block.Body.Blocks) > 0 {
		r.Blocks = decodeBlocks(block.Body.Blocks)
	}

	pp.Println(r)
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
