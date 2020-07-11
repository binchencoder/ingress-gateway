package gengateway

import (
	"errors"
	"fmt"
	"go/format"
	"path"
	"path/filepath"
	"strings"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	plugin "github.com/golang/protobuf/protoc-gen-go/plugin"

	// "github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway/descriptor"
	"github.com/binchencoder/ease-gateway/gateway/protoc-gen-grpc-gateway/descriptor"

	// gen "github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway/generator"
	gen "github.com/binchencoder/ease-gateway/gateway/protoc-gen-grpc-gateway/generator"

	options "github.com/binchencoder/ease-gateway/httpoptions"
)

var (
	errNoTargetService = errors.New("no target service defined in the file")
)

type pathType int

const (
	pathTypeImport pathType = iota
	pathTypeSourceRelative
)

type generator struct {
	reg                *descriptor.Registry
	baseImports        []descriptor.GoPackage
	useRequestContext  bool
	registerFuncSuffix string
	pathType           pathType
	modulePath         string
	allowPatchFeature  bool
}

// New returns a new generator which generates grpc gateway files.
func New(reg *descriptor.Registry, useRequestContext bool, registerFuncSuffix, pathTypeString, modulePathString string, allowPatchFeature bool) gen.Generator {
	var imports []descriptor.GoPackage
	for pkgpath, alias := range map[string]string{
		"context":      "",
		"io":           "",
		"net/http":     "",
		"regexp":       "",
		"strings":      "",
		"sync":         "",
		"unicode/utf8": "",
		"github.com/binchencoder/ease-gateway/gateway/runtime": "",
		"github.com/grpc-ecosystem/grpc-gateway/utilities":     "",
		"github.com/golang/protobuf/proto":                     "",
		"github.com/binchencoder/gateway-proto/data":           "vexpb",
		"github.com/binchencoder/gateway-proto/frontend":       "fpb",
		"github.com/binchencoder/letsgo/grpc":                  "lgr",
		"github.com/binchencoder/skylb-api/balancer":           "",
		"github.com/binchencoder/skylb-api/client":             "",
		"github.com/binchencoder/skylb-api/client/option":      "",
		"github.com/binchencoder/skylb-api/proto":              "skypb",
		"google.golang.org/grpc":                               "",
		"google.golang.org/grpc/codes":                         "",
		"google.golang.org/grpc/naming":                        "",
		"google.golang.org/grpc/grpclog":                       "",
		"google.golang.org/grpc/status":                        "",
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

	var pathType pathType
	switch pathTypeString {
	case "", "import":
		// paths=import is default
	case "source_relative":
		pathType = pathTypeSourceRelative
	default:
		glog.Fatalf(`Unknown path type %q: want "import" or "source_relative".`, pathTypeString)
	}

	return &generator{
		reg:                reg,
		baseImports:        imports,
		useRequestContext:  useRequestContext,
		registerFuncSuffix: registerFuncSuffix,
		pathType:           pathType,
		modulePath:         modulePathString,
		allowPatchFeature:  allowPatchFeature,
	}
}

func (g *generator) Generate(targets []*descriptor.File) ([]*plugin.CodeGeneratorResponse_File, error) {
	var files []*plugin.CodeGeneratorResponse_File
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
		name, err := g.getFilePath(file)
		if err != nil {
			glog.Errorf("%v: %s", err, code)
			return nil, err
		}
		ext := filepath.Ext(name)
		base := strings.TrimSuffix(name, ext)
		output := fmt.Sprintf("%s.pb.gw.go", base)
		files = append(files, &plugin.CodeGeneratorResponse_File{
			Name:    proto.String(output),
			Content: proto.String(string(formatted)),
		})
		glog.V(1).Infof("Will emit %s", output)
	}
	return files, nil
}

func (g *generator) getFilePath(file *descriptor.File) (string, error) {
	name := file.GetName()
	switch {
	case g.modulePath != "" && g.pathType != pathTypeImport:
		return "", errors.New("cannot use module= with paths=")

	case g.modulePath != "":
		trimPath, pkgPath := g.modulePath+"/", file.GoPkg.Path+"/"
		if !strings.HasPrefix(pkgPath, trimPath) {
			return "", fmt.Errorf("%v: file go path does not match module prefix: %v", file.GoPkg.Path, trimPath)
		}
		return filepath.Join(strings.TrimPrefix(pkgPath, trimPath), filepath.Base(name)), nil

	case g.pathType == pathTypeImport && file.GoPkg.Path != "":
		return fmt.Sprintf("%s/%s", file.GoPkg.Path, filepath.Base(name)), nil

	default:
		return name, nil
	}
}

func (g *generator) generate(file *descriptor.File) (string, error) {
	pkgSeen := make(map[string]bool)
	var imports []descriptor.GoPackage
	for _, pkg := range g.baseImports {
		pkgSeen[pkg.Path] = true
		imports = append(imports, pkg)
	}
	for _, svc := range file.Services {
		for _, m := range svc.Methods {
			imports = append(imports, g.addEnumPathParamImports(file, m, pkgSeen)...)
			pkg := m.RequestType.File.GoPkg

			if m.RequestType.HasRule() {
				g.populateImportForValidation(m.RequestType, file, &pkgSeen, &imports)
			}
			if m.Options == nil || !proto.HasExtension(m.Options, options.E_Http) ||
				pkg == file.GoPkg || pkgSeen[pkg.Path] {
				continue
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
