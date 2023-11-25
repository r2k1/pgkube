package main

import (
	"bytes"
	"fmt"
	"io"
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
			"-v", "./tmp/.cache/golangci-lint/v1.55.2:/root/.cache", // cache previous runs
			"-w", "/app",
			"golangci/golangci-lint:v1.55.2",
			"golangci-lint", "run", "-v",
		},
	})
}

func DockerPush() {
	mg.Deps(Generate, Test, LintDocker, DockerLogin)
	checkErr(Exec(Cmd{
		Name: "docker",
		Args: []string{"buildx", "build", "--platform=linux/amd64,linux/arm64,linux/arm/v7", "-t", dockerImage(), "--push", "."},
	}))
}

func DockerLogin() error {
	return Exec(Cmd{
		Name:    "docker",
		Args:    []string{"login", "-u", os.Getenv("DOCKER_LOGIN"), "ghcr.io", "-p", os.Getenv("DOCKER_PASSWORD")},
		NoPrint: true,
	})
}

func KubeApply(image string) {
	checkErr(Exec(Cmd{
		Name: "kubectl",
		Args: []string{"apply", "-f", "./kube/postgres.yaml"},
	}))

	pgKubeYaml, err := os.ReadFile("./kube/pgkube.yaml")
	checkErr(err)

	newPgKubeYaml := strings.Replace(string(pgKubeYaml), "ghcr.io/r2k1/pgkube:latest", image, 1)

	err = Exec(Cmd{
		Name:  "kubectl",
		Args:  []string{"apply", "-f", "-"},
		Stdin: strings.NewReader(newPgKubeYaml),
	})
	checkErr(err)

	checkErr(Exec(Cmd{
		Name: "kubectl",
		Args: []string{"-n", "pgkube", "rollout", "restart", "deployment", "pgkube"},
	}))
}

func KubeLog() error {
	return Exec(Cmd{
		Name: "kubectl",
		Args: []string{"-n", "pgkube", "logs", "-f", "deployment/pgkube"},
	})
}

// Creates a kind cluster with postgres and pgkube
func KindCreate(clusterName string) {
	checkErr(Exec(Cmd{
		Name: "kind",
		Args: []string{"create", "cluster", "--name", clusterName, "--config", "./magefiles/kind.yaml"},
	}))
	checkErr(Exec(Cmd{
		Name: "kubectl",
		Args: []string{"config", "use-context", "kind-" + clusterName},
	}))
}

func dockerImage() string {
	image := os.Getenv("DOCKER_IMAGE")
	if image == "" {
		panic("DOCKER_IMAGE env var is not set")
	}
	return image
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

func DeployKind() {
	image := dockerImage()
	err := Exec(Cmd{
		Name: "docker",
		Args: []string{"build", "-t", image, "--push", "."},
	})
	checkErr(err)
	KubeApply(image)
}

func RecreateKindCluster() {
	clusterName := "pgkube"
	_ = Exec(Cmd{
		Name: "kind",
		Args: []string{"delete", "cluster", "--name", clusterName},
	})
	KindCreate(clusterName)
}

func checkErr(err error) {
	if err != nil {
		panic(err)
	}
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

func ExecOutput(opts Cmd) (string, error) {
	out := &bytes.Buffer{}
	opts.Stdout = out
	err := Exec(opts)
	return out.String(), err
}
