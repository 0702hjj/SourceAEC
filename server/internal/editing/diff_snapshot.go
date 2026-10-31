// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"ifcviewer/server/internal/resourcebudget"
	"os"
)

type versionInput struct {
	data   []byte
	script string
	inputs map[string]string
}

// Comparing stored products uses bounded memory but no geometry worker.
var productComparisons = make(chan struct{}, 3)
var comparisonBytes = resourcebudget.NewBytes(512 << 20)

func (o *operation) captureVersion(name string, reserve func(int64) error) (versionInput, error) {
	key := "uploads/" + o.model + "." + o.service.Kind
	if name != "current" {
		if !versionPattern.MatchString(name) {
			return versionInput{}, failure(422, "invalid version")
		}
		key = o.key("scripts/" + name + ".product")
	}
	size, err := o.runtime.StoredArtifactSize(o.ctx, o.service.Root, o.tx, key)
	if err == nil {
		if err = reserve(size); err != nil {
			return versionInput{}, err
		}
		data, err := o.read(key)
		return versionInput{data: data}, err
	}
	if !os.IsNotExist(err) || name == "current" {
		return versionInput{}, err
	}
	// A legacy execution may produce any allowed product size. Reserve its
	// worst-case payload before launching it; do not learn the size too late.
	if err = reserve(productLimit); err != nil {
		return versionInput{}, err
	}
	script, err := o.versionScript(name)
	if err != nil {
		return versionInput{}, err
	}
	o.replayVersion = name
	inputs, err := o.inputs()
	return versionInput{script: script, inputs: inputs}, err
}

func (o *operation) detachedDiff(body []byte) (any, error) {
	var releases []func()
	defer func() {
		for _, release := range releases {
			release()
		}
	}()
	reserve := func(n int64) error {
		release, ok := comparisonBytes.Try(n)
		if !ok {
			return failure(429, "comparison byte capacity exhausted")
		}
		releases = append(releases, release)
		return nil
	}
	var in struct {
		Base   string `json:"base"`
		Target string `json:"target"`
	}
	if err := decodeBody(body, &in); err != nil {
		return nil, err
	}
	if in.Base == "" || in.Target == "" {
		return nil, failure(422, "base and target required")
	}
	select {
	case productComparisons <- struct{}{}:
		defer func() { <-productComparisons }()
	default:
		return nil, failure(429, "comparison capacity exhausted")
	}
	if err := o.prepareVersionRead(); err != nil {
		return nil, err
	}
	o.readOnly = true
	release, err := o.begin()
	if err != nil {
		return nil, err
	}
	defer release()
	defer func() {
		if o.tx != "" {
			_, _ = o.call("rollback", nil)
		}
	}()
	a, err := o.captureVersion(in.Base, reserve)
	if err != nil {
		return nil, err
	}
	b, err := o.captureVersion(in.Target, reserve)
	if err != nil {
		return nil, err
	}
	if _, err = o.call("commit", nil); err != nil {
		return nil, err
	}
	o.tx = ""
	if a.data == nil || b.data == nil {
		select {
		case buildLifecycles <- struct{}{}:
			defer func() { <-buildLifecycles }()
		default:
			return nil, failure(429, "build capacity exhausted")
		}
	}
	for _, v := range []*versionInput{&a, &b} {
		if v.data == nil {
			product, e := o.service.Executor.Run(context.WithValue(o.ctx, inputContextKey{}, v.inputs), v.script, o.service.Kind)
			if e != nil {
				return nil, e
			}
			if len(product.Data) > productLimit {
				return nil, failure(422, "invalid product size")
			}
			v.data = product.Data
		}
	}
	result, err := Compare(a.data, b.data, o.service.Kind)
	if err != nil {
		return nil, failure(422, err.Error())
	}
	result["base"], result["target"] = in.Base, in.Target
	return result, nil
}
