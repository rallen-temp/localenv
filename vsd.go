package main

import (
	"embed"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"rsc.io/quote"
)

func run(msg string, cmd *exec.Cmd) {
	fmt.Println(msg)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout
	if err := cmd.Run(); err != nil {
		fmt.Println("Error:", err)
	}
	fmt.Println("")
}

// Sets up the shared Docker Compose network.
//
// Creates the named (attachable) network if it doesn't exist.
func setupNetwork(composeNetwork string) {
	networks := exec.Command("docker", "network", "ls")
	output, err := networks.Output()
	if err != nil {
		log.Println(err)
	}

	matches := regexp.MustCompile(composeNetwork).FindStringSubmatch(string(output))
	if len(matches) >= 1 {
		fmt.Printf("Docker network %s already exists, joining.\n", composeNetwork)
	} else {
		fmt.Println("Create user-defined network")
		createNewnet := exec.Command("docker", "network", "create", "--driver", "bridge", "--attachable", composeNetwork)
		if output, err := createNewnet.Output(); err != nil {
			fmt.Println("Error:", err)
		} else {
			fmt.Printf("Otuput: %s\n", output)
		}
	}
}

func showHelp() {
	fmt.Println("Help placeholder")
}

// Project ..  Environment variables used by Docker Compose.
type Project struct {
	// Path to Docker Compose specifications.
	composeSpecs string
	// Shared network name.
	network string
	// Path to source code directory, mounted into containers.
	source string
	// Name of current directory, used for service aliases.
	name string
	// Expected input by the php-fpm service, tells XDebug address where to find IDE.
	// php-fpm service located in docker-compose.vsd.yml file.
	// https://www.reddit.com/r/bashonubuntuonwindows/comments/c871g7/command_to_get_virtual_machine_ip_in_wsl2/
	xdebug string
}

// Gather information used by all sub-commands.
func gatherPrerequisites() Project {
	composeNetwork := `VSD`

	projectSource, err := os.Getwd()
	if err != nil {
		log.Println(err)
	}
	fmt.Printf("Your project location is %s\n", projectSource)

	// https://stackoverflow.com/a/1371283
	projectName, err := exec.Command("bash", "-c", "echo ${PWD##*/}").Output()
	if err != nil {
		fmt.Println("Error:", err)
	} else {
		fmt.Printf("Your project name is: %s", projectName)
	}

	xdebugHost, err := exec.Command("bash", "-c", `ip addr show eth0 | grep -oE '\d+(\.\d+){3}' | head -n 1`).Output()
	if err != nil {
		fmt.Println("Error:", err)
	} else {
		fmt.Printf("XDebug will contact your Visual Studio Code IDE at %s\n", xdebugHost)
	}

	return Project{"docker", composeNetwork, projectSource, strings.TrimSuffix(string(projectName), "\n"), string(xdebugHost)}
}

func main() {
	fmt.Println("WELCOME TO THE VSD ENVIRONMENT !!!")
	fmt.Println("(V)isual Studio Code | (S)ubsystem4Linux | (D)ocker")
	fmt.Println("")

	if len(os.Args) == 1 {
		showHelp()
		os.Exit(0)
	}

	project := gatherPrerequisites()
	setupNetwork(project.network)

	// Set up environment variables for Docker Compose.
	os.Setenv("COMPOSE_NETWORK", project.network)
	os.Setenv("PROJECT_SOURCE", project.source)
	os.Setenv("PROJECT_NAME", project.name)
	os.Setenv("XDEBUG_REMOTE_HOST", project.xdebug)

	switch os.Args[1] {
	case "version":
		print("VSD version 0.3.0\n")
	case "status":
		stackStatus(project)
	case "start":
		startShared(project)
		startProject(project)
		stackStatus(project)
	case "down":
		stackDown(project)
	case "recreate":
	case "rec":
		stackDown(project)
		startShared(project)
		startProject(project)
		stackStatus(project)
	case "show":
		//@TODO: Create a mapping of services source ports, user should not need to specify them.
		serviceShow(project)
	case "open":
		servicePort := serviceShow(project)
		serviceOpen(servicePort)
	// @TODO: Provide override subcommand, emits physical compose override file from embed compose file. Provide directory listing of available overrides.
	default:
		showHelp()
	}

	fmt.Println(quote.Go())
}

//go:embed docker
var dockerfs embed.FS

func embedRead(filename string) []byte {
	file, e := dockerfs.ReadFile(filename)
	if e != nil {
		panic(e)
	}
	return file
}

// Execute Docker Compose command using embedded spec.
//
// For pipe execution see https://golang.org/pkg/os/exec/#Cmd.StdinPipe.
func dockerComposeEmbed(projectName string, specName string, command string) {
	// @TODO: HOW TO ALLOW AN OVERRIDE FILE TO BE INCLUDED?
	// @todo: add project name to shared stack.
	// @todo: switch all exec to embedded: allows calling binary globally.

	specFile := embedRead(specName)
	// // project_stack := embed_read("run/drupal/docker-compose.vsd.yml")
	cmd := exec.Command("bash", "-c",
		fmt.Sprintf("docker-compose --project-name %s --file /dev/stdin %s", projectName, command))

	stdin, err := cmd.StdinPipe()
	if err != nil {
		fmt.Println("Error:", err)
	}

	go func() {
		defer stdin.Close()
		io.WriteString(stdin, string(specFile))
	}()

	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println("Error:", err)
	}

	fmt.Printf("%s\n", out)
}

// Show current stack status.
func stackStatus(project Project) {

	fmt.Println("Shared services status")
	dockerComposeEmbed("localenv", fmt.Sprintf("%s/docker-compose.shared.yml", project.composeSpecs), "ps")

	fmt.Println("Project services status")
	dockerComposeEmbed(project.name, fmt.Sprintf("%s/run/drupal/docker-compose.vsd.yml", project.composeSpecs), "ps")
}

// Start compose service for current directory.
func startProject(project Project) {
	fmt.Println("Start project services")
	dockerComposeEmbed(project.name, "docker/run/drupal/docker-compose.vsd.yml", "up --detach")
}

// Fire up stack shared amongst all projects.
func startShared(project Project) {
	fmt.Println("Start shared services")
	dockerComposeEmbed("localenv", "docker/docker-compose.shared.yml", "up --detach --no-recreate")
}

// Remove services, containers, and networks.
func stackDown(project Project) {
	fmt.Println("Stop shared services")
	dockerComposeEmbed("localenv", "docker/docker-compose.shared.yml", "down --remove-orphans")

	fmt.Println("Stop project servicess")
	dockerComposeEmbed(project.name, "docker/run/drupal/docker-compose.vsd.yml", "down --remove-orphans")

	run("Cleanup Docker containers",
		exec.Command("docker", "system", "prune", "--force"))

	run("Cleanup Docker network",
		exec.Command("docker", "network", "rm", project.network))
}

// Show location of service port.
//
// Example: go run ./vsd.go show nginx 8080
func serviceShow(project Project) string {
	// @TODO: Decouple domain-name for use with let's encrypt!

	var service string
	var port string

	// Define default service to show.
	var command string
	if len(os.Args) >= 3 && os.Args[2] != "" && os.Args[3] != "" {
		service = os.Args[2]
		port = os.Args[3]
	} else {
		service = "nginx"
		port = "8080"
	}

	fmt.Printf("Retrieving service %s @ %s\n", service, port)

	// NOTE: Only shows project services, and not shared services!
	command = fmt.Sprintf(`docker-compose --project-name="%s" \
	 --file %s/run/drupal/docker-compose.vsd.yml \
	 port %s %s | sed 's/0.0.0.0/%s/g'`,
		strings.TrimSuffix(project.name, "\n"),
		project.composeSpecs,
		service,
		port,
		"localhost")

	url := exec.Command("bash", "-c", command)

	srvLocation, err := url.Output()
	if err != nil {
		fmt.Println("Error:", err)
	} else {
		fmt.Printf("Service %s is running at: %s\n", service, srvLocation)
	}
	return string(srvLocation)
}

// Open default browser to specified services' mapped port.
//
// Example: go run ./vsd.go open nginx 8080
//
// Resources:
// - https://ss64.com/nt/cmd.html
// - https://superuser.com/questions/1182275/how-to-use-start-command-in-bash-on-windows
// - https://github.com/microsoft/terminal/issues/204#issuecomment-696816617
func serviceOpen(servicePort string) {
	format := fmt.Sprintf(`cmd.exe /c start chrome "http://%s" 2> /dev/null`, servicePort)

	command := exec.Command("bash", "-c", format)
	command.Stdout = os.Stdout
	command.Stderr = os.Stdout
	if err := command.Run(); err != nil {
		fmt.Println("Error:", err)
	}
}