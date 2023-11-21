package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/joho/godotenv"
	"github.com/magefile/mage/mg"
)

func init() {
	_ = godotenv.Load("./magefiles/.env")
}

func PostgresRecreate() error {
	// start postgres and seed it with .sql file
	_ = Exec(Cmd{
		Name: "docker",
		Args: []string{"rm", "-f", "kube-postgres"},
	})

	return PostgresStart()
}

func PostgresStart() error {
	return Exec(Cmd{
		Name: "docker",
		Args: []string{"run", "--rm", "-d", "-p", "5432:5432", "--name", "kube-postgres", "-e", "POSTGRES_USER=pgkube-user", "-e", "POSTGRES_PASSWORD=pgkube-password", "postgres:15.4-alpine"},
	})
}

func Generate() error {
	return Exec(Cmd{
		Dir:  "./app",
		Name: "go",
		Args: []string{"generate", "./..."},
	})
}

func PortForward() error {
	return Exec(Cmd{
		Name: "kubectl",
		Args: []string{"port-forward", "sts/postgres", "-n", "pgkube", "5433:5432"},
	})
}

func Run() error {
	return Exec(Cmd{
		Dir:  "./app",
		Name: "go",
		Args: []string{"run", "."},
	})
}

func Test() error {
	return Exec(Cmd{
		Dir:  "./app",
		Name: "go",
		Args: []string{"test", "./..."},
	})
}

func Lint() error {
	return Exec(Cmd{
		Dir:  "./app",
		Name: "golangci-lint",
		Args: []string{"run", "-v"},
	})
}

func LintDocker() error {
	return Exec(Cmd{
		Name: "docker",
		Args: []string{"run", "--rm",
			"-t", // add colors
			"-v", "./app:/app",
			"-v", "./tmp/.cache/golangci-lint/v1.54.2:/root/.cache", // cache previous runs
			"-w", "/app",
			"golangci/golangci-lint:v1.54.2",
			"golangci-lint", "run", "-v",
		},
	})
}

func DockerPush(tag string) error {
	// TODO: check there is no changes after generate
	mg.Deps(Generate, Test, LintDocker, DockerLogin)
	return Exec(Cmd{
		Name: "docker",
		Args: []string{"buildx", "build", "--platform=linux/amd64,linux/arm64,linux/arm/v7", "-t", dockerImage(tag), "--push", "."},
	})
}

func DockerLogin() error {
	return Exec(Cmd{
		Name:    "docker",
		Args:    []string{"login", "-u", os.Getenv("DOCKER_LOGIN"), "ghcr.io", "-p", os.Getenv("DOCKER_PASSWORD")},
		NoPrint: true,
	})
}

func KubeApply(tag string) error {
	if err := Exec(Cmd{
		Name: "kubectl",
		Args: []string{"apply", "-f", "./kube/postgres.yaml"},
	}); err != nil {
		return err
	}

	pgKubeYaml, err := os.ReadFile("./kube/pgkube.yaml")
	if err != nil {
		return err
	}

	newPgKubeYaml := strings.Replace(string(pgKubeYaml), "ghcr.io/r2k1/pgkube:latest", dockerImage(tag), 1)

	if err := Exec(Cmd{
		Name:  "kubectl",
		Args:  []string{"apply", "-f", "-"},
		Stdin: strings.NewReader(newPgKubeYaml),
	}); err != nil {
		return err
	}

	if err := Exec(Cmd{
		Name: "kubectl",
		Args: []string{"-n", "pgkube", "rollout", "restart", "deployment", "pgkube"},
	}); err != nil {
		slog.Error(err.Error())
	}

	return nil
}

func PushAndApply(tag string) error {
	err := DockerPush(tag)
	if err != nil {
		return err
	}
	return KubeApply(tag)
}

func KubeLog() error {
	return Exec(Cmd{
		Name: "kubectl",
		Args: []string{"-n", "pgkube", "logs", "-f", "deployment/pgkube"},
	})
}

const KindClusterName = "pgkube"

// Creates a kind cluster with postgres and pgkube
func KindCreate() error {
	var err error
	err = Exec(Cmd{
		Name: "kind",
		Args: []string{"create", "cluster", "--name", KindClusterName, "--config", "./magefiles/kind.yaml"},
	})
	if err != nil {
		return err
	}
	return Exec(Cmd{
		Name: "kubectl",
		Args: []string{"config", "use-context", "kind-" + KindClusterName},
	})
}

func KindDelete() error {
	return Exec(Cmd{
		Name: "kind",
		Args: []string{"delete", "cluster", "--name", KindClusterName},
	})
}

func dockerImage(tag string) string {
	return fmt.Sprintf("%s:%s", os.Getenv("DOCKER_REGISTRY"), tag)
}

type Cmd struct {
	Dir     string
	Name    string
	Args    []string
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
	NoPrint bool
}

func Exec(opts Cmd) error {
	c := exec.Command(opts.Name, opts.Args...)
	c.Dir = opts.Dir

	if opts.Stdout == nil {
		c.Stdout = os.Stdout
	} else {
		c.Stdout = opts.Stdout
	}
	if opts.Stderr == nil {
		c.Stderr = os.Stderr
	} else {
		c.Stderr = opts.Stderr
	}
	if opts.Stdin != nil {
		c.Stdin = opts.Stdin
	}

	colorReset := "\033[0m"
	colorYellow := "\033[33m"
	colorCyan := "\033[36m"

	if !opts.NoPrint {
		fmt.Printf("⚙️ %s%s%s %s%s%s\n", colorCyan, opts.Dir, colorReset, colorYellow, c.String(), colorReset)
	}
	return c.Run()
}
