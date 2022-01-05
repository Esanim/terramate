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

package generate

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/madlambda/spells/errutil"
	"github.com/mineiros-io/terramate"
	"github.com/mineiros-io/terramate/config"
	"github.com/mineiros-io/terramate/hcl"
	"github.com/mineiros-io/terramate/hcl/eval"
	"github.com/mineiros-io/terramate/project"
)

const (
	// BackendCfgFilename is the name of the terramate generated tf file for backend configuration.
	BackendCfgFilename = "_gen_backend_cfg.tm.tf"

	// LocalsFilename is the name of the terramate generated tf file for exported locals.
	LocalsFilename = "_gen_locals.tm.tf"
)

const (
	ErrBackendConfigGen   errutil.Error = "generating backend config"
	ErrExportingLocalsGen errutil.Error = "generating locals"
	ErrLoadingGlobals     errutil.Error = "loading globals"
	ErrManualCodeExists   errutil.Error = "manually defined code found"
)

// Do will walk all the directories starting from project's root
// generating code for any stack it finds as it goes along.
//
// It will return an error if it finds any invalid Terramate configuration files
// or if it can't generate the files properly for some reason.
//
// The provided root must be the project's root directory as an absolute path.
func Do(root string) error {
	if !filepath.IsAbs(root) {
		return fmt.Errorf("project's root %q must be an absolute path", root)
	}

	info, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("checking project's root directory %q: %v", root, err)
	}

	if !info.IsDir() {
		return fmt.Errorf("project's root %q is not a directory", root)
	}

	metadata, err := terramate.LoadMetadata(root)
	if err != nil {
		return fmt.Errorf("loading metadata: %w", err)
	}

	var errs []error

	for _, stackMetadata := range metadata.Stacks {
		// At the time the most intuitive way was to start from the stack
		// and go up until reaching the root, looking for a config.
		// Basically navigating from the order of precedence, since
		// more specific configuration overrides base configuration.
		// Not the most optimized way (re-parsing), we can improve later
		stackpath := project.AbsPath(root, stackMetadata.Path)

		globals, err := terramate.LoadStackGlobals(root, stackMetadata)
		if err != nil {
			errs = append(errs, fmt.Errorf(
				"stack %q: %w: %v",
				stackpath,
				ErrLoadingGlobals,
				err))
			continue
		}

		evalctx := eval.NewContext(stackpath)

		if err := stackMetadata.SetOnEvalCtx(evalctx); err != nil {
			errs = append(errs, fmt.Errorf("stack %q: %v", stackpath, err))
			continue
		}

		if err := globals.SetOnEvalCtx(evalctx); err != nil {
			errs = append(errs, fmt.Errorf("stack %q: %v", stackpath, err))
			continue
		}

		if err := generateStackBackendConfig(root, stackpath, evalctx); err != nil {
			errs = append(errs, fmt.Errorf("stack %q: generating backend config: %w", stackpath, err))
		}

		if err := generateStackLocals(root, stackpath, stackMetadata, globals); err != nil {
			err = errutil.Chain(ErrExportingLocalsGen, err)
			errs = append(errs, fmt.Errorf("stack %q: %w", stackpath, err))
		}
	}

	// FIXME(katcipis): errutil.Chain produces a very hard to read string representation
	// for this case, we have a possibly big list of errors here, not an
	// actual chain (multiple levels of calls).
	// We do need the error wrapping for the error handling on tests (for now at least).
	if err := errutil.Chain(errs...); err != nil {
		return fmt.Errorf("failed to generate code: %w", err)
	}

	return nil
}

func generateStackLocals(
	rootdir string,
	stackpath string,
	metadata terramate.StackMetadata,
	globals *terramate.Globals,
) error {
	genfile := filepath.Join(stackpath, LocalsFilename)
	if err := checkFileCanBeOverwritten(genfile); err != nil {
		return err
	}

	stackLocals, err := terramate.LoadStackExportedLocals(rootdir, metadata, globals)
	if err != nil {
		return err
	}

	localsAttrs := stackLocals.Attributes()
	if len(localsAttrs) == 0 {
		return nil
	}

	sortedAttrs := make([]string, 0, len(localsAttrs))
	for name := range localsAttrs {
		sortedAttrs = append(sortedAttrs, name)
	}
	// Avoid generating code with random attr order (map iteration is random)
	sort.Strings(sortedAttrs)

	gen := hclwrite.NewEmptyFile()
	body := gen.Body()
	localsBlock := body.AppendNewBlock("locals", nil)
	localsBody := localsBlock.Body()

	for _, name := range sortedAttrs {
		localsBody.SetAttributeValue(name, localsAttrs[name])
	}

	tfcode := AddHeader(gen.Bytes())
	return os.WriteFile(genfile, tfcode, 0666)
}

func generateStackBackendConfig(root string, stackpath string, evalctx *eval.Context) error {
	genfile := filepath.Join(stackpath, BackendCfgFilename)
	if err := checkFileCanBeOverwritten(genfile); err != nil {
		return err
	}

	tfcode, err := loadStackBackendConfig(root, stackpath, evalctx)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBackendConfigGen, err)
	}

	if len(tfcode) == 0 {
		return nil
	}

	return os.WriteFile(genfile, tfcode, 0666)
}

func loadStackBackendConfig(root string, configdir string, evalctx *eval.Context) ([]byte, error) {
	if !strings.HasPrefix(configdir, root) {
		// check if we are outside of project's root, time to stop
		return nil, nil
	}

	configfile := filepath.Join(configdir, config.Filename)
	if _, err := os.Stat(configfile); err != nil {
		return loadStackBackendConfig(root, filepath.Dir(configdir), evalctx)
	}

	config, err := os.ReadFile(configfile)
	if err != nil {
		return nil, fmt.Errorf("reading config: %v", err)
	}

	parsedConfig, err := hcl.Parse(configfile, config)
	if err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	parsed := parsedConfig.Terramate
	if parsed == nil || parsed.Backend == nil {
		return loadStackBackendConfig(root, filepath.Dir(configdir), evalctx)
	}

	gen := hclwrite.NewEmptyFile()
	rootBody := gen.Body()
	tfBlock := rootBody.AppendNewBlock("terraform", nil)
	tfBody := tfBlock.Body()
	backendBlock := tfBody.AppendNewBlock(parsed.Backend.Type, parsed.Backend.Labels)
	backendBody := backendBlock.Body()

	if err := copyBody(backendBody, parsed.Backend.Body, evalctx); err != nil {
		return nil, err
	}

	return AddHeader(gen.Bytes()), nil
}

// AddHeader will add a proper Terramate header indicating that code
// was generated by Terramate.
func AddHeader(code []byte) []byte {
	return append([]byte(codeHeader+"\n\n"), code...)
}

const codeHeader = "// GENERATED BY TERRAMATE: DO NOT EDIT"

func copyBody(target *hclwrite.Body, src *hclsyntax.Body, evalctx *eval.Context) error {
	if src == nil || target == nil {
		return nil
	}

	// Avoid generating code with random attr order (map iteration is random)
	attrs := sortedAttributes(src.Attributes)

	for _, attr := range attrs {
		val, err := evalctx.Eval(attr.Expr)
		if err != nil {
			return fmt.Errorf("parsing attribute %q: %v", attr.Name, err)
		}
		target.SetAttributeValue(attr.Name, val)
	}

	for _, block := range src.Blocks {
		targetBlock := target.AppendNewBlock(block.Type, block.Labels)
		targetBody := targetBlock.Body()
		if err := copyBody(targetBody, block.Body, evalctx); err != nil {
			return err
		}
	}

	return nil
}

func sortedAttributes(attrs hclsyntax.Attributes) []*hclsyntax.Attribute {
	names := make([]string, 0, len(attrs))

	for name := range attrs {
		names = append(names, name)
	}

	sort.Strings(names)

	sorted := make([]*hclsyntax.Attribute, len(names))
	for i, name := range names {
		sorted[i] = attrs[name]
	}

	return sorted
}

func checkFileCanBeOverwritten(path string) error {
	_, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("unsafe to overwrite file, can't stat %q", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("unsafe to overwrite file, can't read %q", path)
	}

	code := string(data)
	if !strings.HasPrefix(code, codeHeader) {
		return fmt.Errorf("%w: at %q", ErrManualCodeExists, path)
	}

	return nil
}