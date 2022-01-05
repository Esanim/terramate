// Copyright 2021 Mineiros GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package hclwrite aims to provide some facilities making it easier/safer
// to generate HCL code for testing purposes. It aims at:
//
// - Close to how HCL is written.
// - Provide formatted string representation.
// - Avoid issues when raw HCL strings are used on tests in general.
//
// It is not a replacement to hclwrite: https://pkg.go.dev/github.com/hashicorp/hcl/v2/hclwrite
// It is just easier/nicer to use on tests + circumvents some limitations like:
//
// - https://stackoverflow.com/questions/67945463/how-to-use-hcl-write-to-set-expressions-with
package hclwrite

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

type Block struct {
	name        string
	labels      []string
	children    []*Block
	expressions map[string]string
	// Not cool to keep 2 copies of values but casting around
	// cty values is quite annoying, so this is a lazy solution.
	ctyvalues map[string]cty.Value
	values    map[string]interface{}
}

type HCL struct {
	blocks []*Block
}

func (b *Block) AddLabel(name string) {
	b.labels = append(b.labels, fmt.Sprintf("%q", name))
}

func (b *Block) AddExpr(key string, expr string) {
	b.expressions[key] = expr
}

func (b *Block) AddNumberInt(key string, v int64) {
	b.ctyvalues[key] = cty.NumberIntVal(v)
	b.values[key] = v
}

func (b *Block) AddString(key string, v string) {
	b.ctyvalues[key] = cty.StringVal(v)
	b.values[key] = fmt.Sprintf("%q", v)
}

func (b *Block) AddBoolean(key string, v bool) {
	b.ctyvalues[key] = cty.BoolVal(v)
	b.values[key] = v
}

func (b *Block) AddBlock(child *Block) {
	b.children = append(b.children, child)
}

func (b *Block) AttributesValues() map[string]cty.Value {
	return b.ctyvalues
}

func (b *Block) HasExpressions() bool {
	return len(b.expressions) > 0
}

func (b *Block) Build(parent *Block) {
	parent.AddBlock(b)
}

func (b *Block) String() string {
	code := b.name + strings.Join(b.labels, " ") + "{\n"
	// Tried properly using hclwrite, it doesnt work well with expressions:
	// - https://stackoverflow.com/questions/67945463/how-to-use-hcl-write-to-set-expressions-with
	for _, name := range b.sortedExpressions() {
		code += fmt.Sprintf("%s=%s\n", name, b.expressions[name])
	}
	for _, name := range b.sortedValues() {
		code += fmt.Sprintf("%s=%v\n", name, b.values[name])
	}
	for _, childblock := range b.children {
		code += childblock.String() + "\n"
	}
	code += "}"
	return Format(code)
}

func (h HCL) String() string {
	strs := make([]string, len(h.blocks))
	for i, block := range h.blocks {
		strs[i] = block.String()
	}
	return strings.Join(strs, "\n")
}

func NewBlock(name string) *Block {
	return &Block{
		name:        name,
		expressions: map[string]string{},
		ctyvalues:   map[string]cty.Value{},
		values:      map[string]interface{}{},
	}
}

func NewHCL(blocks ...*Block) HCL {
	return HCL{blocks: blocks}
}

type BlockBuilder interface {
	Build(*Block)
}

type BlockBuilderFunc func(*Block)

func BuildBlock(name string, builders ...BlockBuilder) *Block {
	b := NewBlock(name)
	for _, builder := range builders {
		builder.Build(b)
	}
	return b
}

func Labels(labels ...string) BlockBuilder {
	return BlockBuilderFunc(func(g *Block) {
		for _, label := range labels {
			g.AddLabel(label)
		}
	})
}

func Expression(key string, expr string) BlockBuilder {
	return BlockBuilderFunc(func(g *Block) {
		g.AddExpr(key, expr)
	})
}

func String(key string, val string) BlockBuilder {
	return BlockBuilderFunc(func(g *Block) {
		g.AddString(key, val)
	})
}

func Boolean(key string, val bool) BlockBuilder {
	return BlockBuilderFunc(func(g *Block) {
		g.AddBoolean(key, val)
	})
}

func NumberInt(key string, val int64) BlockBuilder {
	return BlockBuilderFunc(func(g *Block) {
		g.AddNumberInt(key, val)
	})
}

func Format(code string) string {
	return strings.Trim(string(hclwrite.Format([]byte(code))), "\n ")
}

func (builder BlockBuilderFunc) Build(b *Block) {
	builder(b)
}

func (b *Block) sortedExpressions() []string {
	keys := make([]string, 0, len(b.expressions))
	for k := range b.expressions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (b *Block) sortedValues() []string {
	keys := make([]string, 0, len(b.values))
	for k := range b.values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}