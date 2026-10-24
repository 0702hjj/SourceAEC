// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const productLimit = 256 << 20
const outputLimit = 1 << 20

type Product struct {
	Data, Map   []byte
	Environment string
}
type Executor interface {
	Run(context.Context, string, string) (Product, error)
}
type Sandbox struct {
	Python   string
	UV       string
	CacheDir string
	Timeout  time.Duration
	gate     chan struct{}
}

func NewSandbox() *Sandbox {
	cache, _ := os.UserCacheDir()
	return &Sandbox{Python: os.Getenv("SOURCEAEC_PYTHON"), UV: "uv", CacheDir: filepath.Join(cache, "sourceaec", "environments"), Timeout: 60 * time.Second, gate: make(chan struct{}, 3)}
}

type limitedLog struct {
	mu     sync.Mutex
	data   []byte
	total  int
	cancel context.CancelFunc
}

func (b *limitedLog) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	b.total += n
	b.data = append(b.data, p...)
	if len(b.data) > 2048 {
		b.data = b.data[len(b.data)-2048:]
	}
	if b.total > outputLimit {
		b.cancel()
	}
	return n, nil
}
func command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = time.Second
	return cmd
}
func execute(ctx context.Context, cmd *exec.Cmd) (string, error) {
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	log := &limitedLog{cancel: cancel}
	cmd.Stdout = log
	cmd.Stderr = log
	// The caller's command already observes ctx; a log overflow kills its group.
	err := cmd.Start()
	if err != nil {
		return "", err
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-child.Done():
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		case <-done:
		}
	}()
	err = cmd.Wait()
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	close(done)
	log.mu.Lock()
	defer log.mu.Unlock()
	if log.total > outputLimit {
		return string(log.data), fmt.Errorf("process output limit exceeded")
	}
	return string(log.data), err
}
func (s *Sandbox) Run(ctx context.Context, script, kind string) (Product, error) {
	if kind != "ifc" && kind != "dxf" {
		return Product{}, failure(422, "unsupported product kind")
	}
	bwrap, e := exec.LookPath("bwrap")
	if e != nil {
		return Product{}, failure(503, "bubblewrap is required for script execution")
	}
	prlimit, e := exec.LookPath("prlimit")
	if e != nil {
		return Product{}, failure(503, "prlimit is required for resource limits")
	}
	select {
	case s.gate <- struct{}{}:
		defer func() { <-s.gate }()
	case <-ctx.Done():
		return Product{}, ctx.Err()
	default:
		return Product{}, failure(429, "script concurrency limit reached")
	}
	python, identity, e := s.environment(ctx, script, kind)
	if e != nil {
		return Product{}, e
	}
	directory, e := os.MkdirTemp("", "sourceaec-run-")
	if e != nil {
		return Product{}, e
	}
	defer os.RemoveAll(directory)
	if e = os.WriteFile(filepath.Join(directory, "script.py"), []byte(script), 0600); e != nil {
		return Product{}, e
	}
	inputDir, e := projectInputs(ctx)
	if e != nil {
		return Product{}, e
	}
	defer os.RemoveAll(inputDir)
	argv := []string{"--die-with-parent", "--new-session", "--unshare-all", "--cap-drop", "ALL", "--clearenv", "--setenv", "PATH", "/usr/bin:/bin", "--setenv", "LANG", "C.UTF-8", "--setenv", "HOME", "/tmp", "--setenv", "PYTHONNOUSERSITE", "1", "--setenv", "PYTHONDONTWRITEBYTECODE", "1", "--setenv", "OPENBLAS_NUM_THREADS", "1", "--setenv", "OMP_NUM_THREADS", "1", "--setenv", "MKL_NUM_THREADS", "1", "--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp"}
	for _, path := range []string{"/usr", "/bin", "/lib", "/lib64", "/etc/ssl"} {
		if _, e := os.Stat(path); e == nil {
			argv = append(argv, "--ro-bind", path, path)
		}
	}
	envDir := filepath.Dir(filepath.Dir(python))
	if !strings.HasPrefix(envDir, "/usr") && envDir != "/" {
		argv = append(argv, "--ro-bind", envDir, envDir)
	}
	resolved, _ := filepath.EvalSymlinks(python)
	if resolved != "" && !strings.HasPrefix(resolved, "/usr/") && !strings.HasPrefix(resolved, envDir+"/") {
		runtimeDir := filepath.Dir(filepath.Dir(resolved))
		argv = append(argv, "--ro-bind", runtimeDir, runtimeDir)
	}
	argv = append(argv, "--ro-bind", inputDir, "/inputs", "--setenv", "SOURCEAEC_INPUTS_DIR", "/inputs", "--setenv", "VIEWER_DATA_DIR", "/inputs", "--bind", directory, "/work", "--chdir", "/work", "--setenv", "SOURCEAEC_MAP_OUT", "/work/out.map.json", "--", prlimit, "--as=1073741824", "--nproc=256", "--fsize=268435456", "--", python, "-I", "-B", "/work/script.py", "/work/out."+kind)
	runCtx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	cmd := command(runCtx, bwrap, argv...)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "LANG=C.UTF-8"}
	log, e := execute(runCtx, cmd)
	if e != nil {
		if runCtx.Err() != nil {
			return Product{}, failure(504, "script execution timed out or canceled")
		}
		return Product{}, failure(422, "script execution failed: "+log)
	}
	data, e := readProduct(filepath.Join(directory, "out."+kind), productLimit)
	if e != nil {
		return Product{}, failure(422, "missing or invalid product: "+e.Error())
	}
	if len(data) == 0 {
		return Product{}, failure(422, "empty product")
	}
	mapData, e := readProduct(filepath.Join(directory, "out.map.json"), productLimit)
	if e != nil && !os.IsNotExist(e) {
		return Product{}, failure(422, "invalid source map")
	}
	return Product{Data: data, Map: mapData, Environment: identity}, nil
}
func readProduct(path string, limit int64) ([]byte, error) {
	// O_NOFOLLOW protects publication even when a child replaces a path with a link.
	fd, e := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if e != nil {
		return nil, e
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	stat, e := f.Stat()
	if e != nil {
		return nil, e
	}
	if !stat.Mode().IsRegular() || stat.Size() > limit {
		return nil, fmt.Errorf("product must be a regular file within size limit")
	}
	data, e := io.ReadAll(io.LimitReader(f, limit+1))
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("product limit exceeded")
	}
	return data, e
}
