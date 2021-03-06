package fi

import (
	"fmt"
	"github.com/golang/glog"
	"io/ioutil"
	"k8s.io/kops/util/pkg/vfs"
	"os"
	"reflect"
	"strings"
)

type Context struct {
	Tmpdir string

	Target            Target
	Cloud             Cloud
	CAStore           CAStore
	SecretStore       SecretStore
	ClusterConfigBase vfs.Path

	CheckExisting bool

	tasks map[string]Task
}

func NewContext(target Target, cloud Cloud, castore CAStore, secretStore SecretStore, clusterConfigBase vfs.Path, checkExisting bool, tasks map[string]Task) (*Context, error) {
	c := &Context{
		Cloud:             cloud,
		Target:            target,
		CAStore:           castore,
		SecretStore:       secretStore,
		ClusterConfigBase: clusterConfigBase,
		CheckExisting:     checkExisting,
		tasks:             tasks,
	}

	t, err := ioutil.TempDir("", "deploy")
	if err != nil {
		return nil, fmt.Errorf("error creating temporary directory: %v", err)
	}
	c.Tmpdir = t

	return c, nil
}

func (c *Context) AllTasks() map[string]Task {
	return c.tasks
}

func (c *Context) RunTasks(maxAttemptsWithNoProgress int) error {
	e := &executor{
		context: c,
	}
	return e.RunTasks(c.tasks, maxAttemptsWithNoProgress)
}

func (c *Context) Close() {
	glog.V(2).Infof("deleting temp dir: %q", c.Tmpdir)
	if c.Tmpdir != "" {
		err := os.RemoveAll(c.Tmpdir)
		if err != nil {
			glog.Warningf("unable to delete temporary directory %q: %v", c.Tmpdir, err)
		}
	}
}

//func (c *Context) MergeOptions(options Options) error {
//	return c.Options.Merge(options)
//}

func (c *Context) NewTempDir(prefix string) (string, error) {
	t, err := ioutil.TempDir(c.Tmpdir, prefix)
	if err != nil {
		return "", fmt.Errorf("error creating temporary directory: %v", err)
	}
	return t, nil
}

var typeContextPtr = reflect.TypeOf((*Context)(nil))

func (c *Context) Render(a, e, changes Task) error {
	if _, ok := c.Target.(*DryRunTarget); ok {
		return c.Target.(*DryRunTarget).Render(a, e, changes)
	}

	v := reflect.ValueOf(e)
	vType := v.Type()

	targetType := reflect.ValueOf(c.Target).Type()

	var renderer *reflect.Method
	var rendererArgs []reflect.Value

	for i := 0; i < vType.NumMethod(); i++ {
		method := vType.Method(i)
		if !strings.HasPrefix(method.Name, "Render") {
			continue
		}
		match := true

		var args []reflect.Value
		for j := 0; j < method.Type.NumIn(); j++ {
			arg := method.Type.In(j)
			if arg.ConvertibleTo(vType) {
				continue
			}
			if arg.ConvertibleTo(typeContextPtr) {
				args = append(args, reflect.ValueOf(c))
				continue
			}
			if arg.ConvertibleTo(targetType) {
				args = append(args, reflect.ValueOf(c.Target))
				continue
			}
			match = false
			break
		}
		if match {
			if renderer != nil {
				return fmt.Errorf("Found multiple Render methods that could be invokved on %T", e)
			}
			renderer = &method
			rendererArgs = args
		}

	}
	if renderer == nil {
		return fmt.Errorf("Could not find Render method on type %T (target %T)", e, c.Target)
	}
	rendererArgs = append(rendererArgs, reflect.ValueOf(a))
	rendererArgs = append(rendererArgs, reflect.ValueOf(e))
	rendererArgs = append(rendererArgs, reflect.ValueOf(changes))
	glog.V(11).Infof("Calling method %s on %T", renderer.Name, e)
	m := v.MethodByName(renderer.Name)
	rv := m.Call(rendererArgs)
	var rvErr error
	if !rv[0].IsNil() {
		rvErr = rv[0].Interface().(error)
	}
	return rvErr
}
