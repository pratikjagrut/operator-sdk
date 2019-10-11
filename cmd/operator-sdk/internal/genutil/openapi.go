// Copyright 2018 The Operator-SDK Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package genutil

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/operator-framework/operator-sdk/internal/pkg/scaffold"
	"github.com/operator-framework/operator-sdk/internal/pkg/scaffold/input"
	"github.com/operator-framework/operator-sdk/internal/util/k8sutil"
	"github.com/operator-framework/operator-sdk/internal/util/projutil"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	generatorargs "k8s.io/kube-openapi/cmd/openapi-gen/args"
	"k8s.io/kube-openapi/pkg/generators"
)

// OpenAPIGen generates OpenAPI validation specs for all CRD's in dirs.
func OpenAPIGen() error {
	s := &scaffold.Scaffold{}
	cfg, crds, err := preScaffoldSetup()
	if err != nil {
		return err
	}
	for _, crd := range crds {
		g, v, k := crd.Spec.Group, crd.Spec.Version, crd.Spec.Names.Kind
		if v == "" {
			if len(crd.Spec.Versions) != 0 {
				v = crd.Spec.Versions[0].Name
			} else {
				return fmt.Errorf("crd of group %s kind %s has no version", g, k)
			}
		}
		err := doScffolding(g, v, k, s, cfg)
		if err != nil {
			return err
		}
	}

	log.Info("Code-generation complete.")
	return nil
}

// OpenAPIGenWithIgnoreFlag generates OpenAPI validation specs for all CRD's in dirs.
func OpenAPIGenWithIgnoreFlag(ignoreGroups []string) error {
	s := &scaffold.Scaffold{}
	cfg, crds, err := preScaffoldSetup()
	if err != nil {
		return err
	}
	for _, crd := range crds {
		g, v, k := crd.Spec.Group, crd.Spec.Version, crd.Spec.Names.Kind
		if v == "" {
			if len(crd.Spec.Versions) != 0 {
				v = crd.Spec.Versions[0].Name
			} else {
				return fmt.Errorf("crd of group %s kind %s has no version", g, k)
			}
		}
		for _, group := range ignoreGroups {
			if g != group {
				err := doScffolding(g, v, k, s, cfg)
				if err != nil {
					return err
				}
			}
		}
	}

	log.Info("Code-generation complete.")
	return nil
}

func preScaffoldSetup() (*input.Config, []*v1beta1.CustomResourceDefinition, error) {
	projutil.MustInProjectRoot()

	absProjectPath := projutil.MustGetwd()
	repoPkg := projutil.GetGoPkg()

	gvMap, err := k8sutil.ParseGroupSubpackages(scaffold.ApisDir)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse group versions: (%v)", err)
	}
	gvb := &strings.Builder{}
	for g, vs := range gvMap {
		gvb.WriteString(fmt.Sprintf("%s:%v, ", g, vs))
	}

	log.Infof("Running OpenAPI code-generation for Custom Resource group versions: [%v]\n", gvb.String())

	apisPkg := filepath.Join(repoPkg, scaffold.ApisDir)
	fqApis := k8sutil.CreateFQAPIs(apisPkg, gvMap)
	f := func(a string) error { return openAPIGen(a, fqApis) }
	if err = generateWithHeaderFile(f); err != nil {
		return nil, nil, err
	}

	cfg := &input.Config{
		Repo:           repoPkg,
		AbsProjectPath: absProjectPath,
		ProjectName:    filepath.Base(absProjectPath),
	}
	crds, err := k8sutil.GetCRDs(scaffold.CRDsDir)
	if err != nil {
		return nil, nil, err
	}

	return cfg, crds, nil
}

func doScffolding(g, v, k string, s *scaffold.Scaffold, cfg *input.Config) error {
	r, err := scaffold.NewResource(g+"/"+v, k)
	if err != nil {
		return err
	}
	err = s.Execute(cfg, &scaffold.CRD{Resource: r, IsOperatorGo: projutil.IsOperatorGo()})
	if err != nil {
		return err
	}
	return nil
}

func openAPIGen(hf string, fqApis []string) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	flag.Set("logtostderr", "true")
	for _, api := range fqApis {
		api = filepath.FromSlash(api)
		// Use relative API path so the generator writes to the correct path.
		apiPath := "." + string(filepath.Separator) + api[strings.Index(api, scaffold.ApisDir):]
		args, cargs := generatorargs.NewDefaults()
		// Ignore default output base and set our own output path.
		args.OutputBase = ""
		// openapi-gen already generates a "do not edit" comment.
		args.GeneratedByCommentTemplate = ""
		args.InputDirs = []string{apiPath}
		args.OutputFileBaseName = "zz_generated.openapi"
		args.OutputPackagePath = filepath.Join(wd, apiPath)
		args.GoHeaderFilePath = hf
		// Print API rule violations to stdout
		cargs.ReportFilename = "-"
		if err := generatorargs.Validate(args); err != nil {
			return errors.Wrap(err, "openapi-gen argument validation error")
		}

		err := args.Execute(
			generators.NameSystems(),
			generators.DefaultNameSystem(),
			generators.Packages,
		)
		if err != nil {
			return errors.Wrap(err, "openapi-gen generator error")
		}
	}
	return nil
}
