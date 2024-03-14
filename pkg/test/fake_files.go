package test

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/ghodss/yaml"
	"github.com/kubesaw/ksctl/pkg/assets"
	v1 "github.com/openshift/api/template/v1"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
)

type FakeFileCreator func(t *testing.T) (string, []byte)

func FakeFile(path string, content []byte) FakeFileCreator {
	return func(t *testing.T) (string, []byte) {
		return path, content
	}
}

func FakeTemplate(path string, objects ...runtime.Object) FakeFileCreator {
	return FakeTemplateWithParams(path, []string{}, objects...)
}

func FakeTemplateWithParams(path string, requiredParams []string, objects ...runtime.Object) FakeFileCreator {
	return func(t *testing.T) (s string, bytes []byte) {
		tmpl := v1.Template{}
		tmpl.Name = "fake-template"
		for _, object := range objects {
			tmpl.Objects = append(tmpl.Objects, runtime.RawExtension{
				Object: object,
			})
		}

		for _, param := range requiredParams {
			tmpl.Parameters = append(tmpl.Parameters, v1.Parameter{
				Name:     param,
				Required: true,
			})
		}

		content, err := yaml.Marshal(tmpl)
		require.NoError(t, err)
		return path, content
	}
}

func NewFakeFiles(t *testing.T, fakeFiles ...FakeFileCreator) assets.FS {
	files := map[string][]byte{}
	for _, getFile := range fakeFiles {
		path, bytes := getFile(t)
		files[path] = bytes
	}
	return &fakeFS{
		files: files,
	}
}

type fakeFS struct {
	files map[string][]byte
}

var _ assets.FS = &fakeFS{}

func (f *fakeFS) Open(name string) (fs.File, error) {
	if content, found := f.files[name]; found {
		return &fakeFile{
			content: content,
		}, nil
	}
	return nil, fmt.Errorf("file not found: %s", name)
}

func (f *fakeFS) ReadDir(name string) ([]fs.DirEntry, error) {
	result := []fs.DirEntry{}
	for n := range f.files {
		if filepath.Dir(n) == name {
			result = append(result, &fakeDirEntry{
				name: filepath.Base(n),
			})
		}
	}

	return result, nil
}

func (f *fakeFS) ReadFile(name string) ([]byte, error) {
	if content, found := f.files[name]; found {
		return content, nil
	}
	return nil, fmt.Errorf("file not found: %s", name)
}

type fakeFile struct {
	content []byte
}

var _ fs.File = &fakeFile{}

func (f *fakeFile) Read(out []byte) (int, error) {
	copy(out, f.content)
	return len(f.content), nil
}

func (f *fakeFile) Stat() (fs.FileInfo, error) {
	// not implemented
	return nil, nil
}

func (f *fakeFile) Close() error {
	// not implemened
	return nil
}

type fakeDirEntry struct {
	name string
}

var _ fs.DirEntry = &fakeDirEntry{}

func (f *fakeDirEntry) Name() string {
	return f.name
}

func (f *fakeDirEntry) IsDir() bool {
	return false
}

func (f *fakeDirEntry) Type() fs.FileMode {
	return fs.ModePerm
}

func (f *fakeDirEntry) Info() (fs.FileInfo, error) {
	// not implemented
	return nil, nil
}
