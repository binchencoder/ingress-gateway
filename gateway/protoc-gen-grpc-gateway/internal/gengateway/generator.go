package gengateway

import (
	"errors"
	"fmt"
	"go/format"
	"path"

	"github.com/golang/glog"
	// "github.com/grpc-ecosystem/grpc-gateway/v2/internal/descriptor"
	"github.com/binchencoder/janus-gateway/gateway/internal/descriptor"
	// gen "github.com/grpc-ecosystem/grpc-gateway/v2/internal/generator"
	gen "github.com/binchencoder/janus-gateway/gateway/internal/generator"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/pluginpb"
)

var (
	errNoTargetService = errors.New("no target service defined in the file")
)

type generator struct {
	reg                *descriptor.Registry
	baseImports        []descriptor.GoPackage
	useRequestContext  bool
	registerFuncSuffix string
	allowPatchFeature  bool
	standalone         bool
}

// New returns a new generator which generates grpc gateway files.
func New(reg *descriptor.Registry, useRequestContext bool, registerFuncSuffix string,
	allowPatchFeature, standalone bool) gen.Generator {
	var imports []descriptor.GoPackage
	for pkgpath, alias := range map[string]string{
		"context":      "",
		"io":           "",
		"net/http":     "",
		"regexp":       "",
		"strings":      "",
		"sync":         "",
		"unicode/utf8": "",
		// "github.com/grpc-ecosystem/grpc-gateway/v2/runtime",
		"github.com/binchencoder/janus-gateway/gateway/runtime": "",
		"github.com/grpc-ecosystem/grpc-gateway/v2/utilities":  "",
		"google.golang.org/protobuf/proto":                     "",
		"github.com/binchencoder/gateway-proto/data":           "vexpb",
		"github.com/binchencoder/gateway-proto/frontend":       "fpb",
		"github.com/binchencoder/letsgo/grpc":                  "lgr",
		// "github.com/binchencoder/skylb-api/balancer":           "",
		"github.com/binchencoder/skylb-api/client": "",
		// "github.com/binchencoder/skylb-api/client/option": "",
		"github.com/binchencoder/skylb-api/proto": "skypb",
		"google.golang.org/grpc":                  "",
		"google.golang.org/grpc/codes":            "",
		"google.golang.org/grpc/grpclog":          "",
		"google.golang.org/grpc/metadata":         "",
		"google.golang.org/grpc/status":           "",
	} {
		pkg := descriptor.GoPackage{
			Path:  pkgpath,
			Name:  path.Base(pkgpath),
			Alias: alias,
		}
		if alias == "" {
			if err := reg.ReserveGoPackageAlias(pkg.Name, pkg.Path); err != nil {
				for i := 0; ; i++ {
					alias := fmt.Sprintf("%s_%d", pkg.Name, i)
					if err := reg.ReserveGoPackageAlias(alias, pkg.Path); err != nil {
						continue
					}
					pkg.Alias = alias
					break
				}
			}
		}
		imports = append(imports, pkg)
	}

	return &generator{
		reg:                reg,
		baseImports:        imports,
		useRequestContext:  useRequestContext,
		registerFuncSuffix: registerFuncSuffix,
		allowPatchFeature:  allowPatchFeature,
		standalone:         standalone,
	}
}

func (g *generator) Generate(targets []*descriptor.File) ([]*descriptor.ResponseFile, error) {
	var files []*descriptor.ResponseFile
	for _, file := range targets {
		glog.V(1).Infof("Processing %s", file.GetName())

		code, err := g.generate(file)
		if err == errNoTargetService {
			glog.V(1).Infof("%s: %v", file.GetName(), err)
			continue
		}
		if err != nil {
			return nil, err
		}
		formatted, err := format.Source([]byte(code))
		if err != nil {
			glog.Errorf("%v: %s", err, code)
			return nil, err
		}
		files = append(files, &descriptor.ResponseFile{
			GoPkg: file.GoPkg,
			CodeGeneratorResponse_File: &pluginpb.CodeGeneratorResponse_File{
				Name:    proto.String(file.GeneratedFilenamePrefix + ".pb.gw.go"),
				Content: proto.String(string(formatted)),
			},
		})
	}
	return files, nil
}

func (g *generator) generate(file *descriptor.File) (string, error) {
	pkgSeen := make(map[string]bool)
	var imports []descriptor.GoPackage
	for _, pkg := range g.baseImports {
		pkgSeen[pkg.Path] = true
		imports = append(imports, pkg)
	}

	if g.standalone {
		imports = append(imports, file.GoPkg)
	}

	for _, svc := range file.Services {
		for _, m := range svc.Methods {
			imports = append(imports, g.addEnumPathParamImports(file, m, pkgSeen)...)
			pkg := m.RequestType.File.GoPkg

			if m.RequestType.HasRule() {
				g.populateImportForValidation(m.RequestType, file, &pkgSeen, &imports)
			}
			if len(m.Bindings) == 0 ||
				pkg == file.GoPkg || pkgSeen[pkg.Path] {
				continue
			}
			pkgSeen[pkg.Path] = true
			imports = append(imports, pkg)
		}
	}
	params := param{
		File:               file,
		Imports:            imports,
		UseRequestContext:  g.useRequestContext,
		RegisterFuncSuffix: g.registerFuncSuffix,
		AllowPatchFeature:  g.allowPatchFeature,
	}
	if g.reg != nil {
		params.OmitPackageDoc = g.reg.GetOmitPackageDoc()
	}
	return applyTemplate(params, g.reg)
}

// addEnumPathParamImports handles adding import of enum path parameter go packages
func (g *generator) addEnumPathParamImports(file *descriptor.File, m *descriptor.Method, pkgSeen map[string]bool) []descriptor.GoPackage {
	var imports []descriptor.GoPackage
	for _, b := range m.Bindings {
		for _, p := range b.PathParams {
			e, err := g.reg.LookupEnum("", p.Target.GetTypeName())
			if err != nil {
				continue
			}
			pkg := e.File.GoPkg
			if pkg == file.GoPkg || pkgSeen[pkg.Path] {
				continue
			}
			pkgSeen[pkg.Path] = true
			imports = append(imports, pkg)
		}
	}
	return imports
}

func (g *generator) populateImportForValidation(m *descriptor.Message, file *descriptor.File, pkgSeen *map[string]bool, imports *[]descriptor.GoPackage) {
	for _, f := range m.Fields {
		if f.FieldMessage != nil && f.FieldMessage.HasRule() {
			pkg := f.FieldMessage.File.GoPkg
			if pkg == file.GoPkg {
				continue
			}
			if (*pkgSeen)[pkg.Path] {
				continue
			}
			(*pkgSeen)[pkg.Path] = true
			*imports = append(*imports, pkg)
		}
	}
}
