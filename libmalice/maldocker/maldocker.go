package maldocker

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	log "github.com/Sirupsen/logrus"
	"github.com/maliceio/malice/config"
	// "github.com/maliceio/malice/libmalice/maldocker/utils"
	"github.com/fsouza/go-dockerclient"
)

// NOTE: https://github.com/eris-ltd/eris-cli/blob/master/perform/docker_run.go

var (
	endpoint    = os.Getenv("DOCKER_HOST")
	path        = os.Getenv("DOCKER_CERT_PATH")
	ca          = fmt.Sprintf("%s/ca.pem", path)
	cert        = fmt.Sprintf("%s/cert.pem", path)
	key         = fmt.Sprintf("%s/key.pem", path)
	client      *docker.Client
	clientError error
	ip          string
	port        string
)

func init() {
	var err error
	if config.Conf.Environment.Run == "production" {
		// Log as JSON instead of the default ASCII formatter.
		log.SetFormatter(&log.JSONFormatter{})
		// Only log the warning severity or above.
		log.SetLevel(log.InfoLevel)
		// log.SetFormatter(&logstash.LogstashFormatter{Type: "malice"})
	} else {
		// Log as ASCII formatter.
		log.SetFormatter(&log.TextFormatter{})
		// Only log the warning severity or above.
		log.SetLevel(log.DebugLevel)
	}
	// Output to stderr instead of stdout, could also be a file.
	log.SetOutput(os.Stdout)

	if endpoint == "" {
		endpoint = config.Conf.Docker.EndPoint
	}

	ip, port, err = parseDockerEndoint(endpoint)
	if err != nil {
		log.Error(err)
	}

	client, clientError = docker.NewTLSClient(endpoint, cert, key, ca)

	// Make sure we can connect to the docker client
	if clientError != nil {
		handleClientError()
	}
}

// assert error
func assert(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

//GetIP returns IP of docker client
func GetIP() string {
	return ip
}

// StartContainer starts a malice docker container
func StartContainer(name string, image string, logs bool) (cont *docker.Container, err error) {

	assert(PingDockerClient(client))

	_, exists, err := ContainerExists(client, name)

	if exists {
		log.WithFields(log.Fields{
			"exisits": exists,
			"name":    name,
			// "id":      elkContainer.ID,
			"env": config.Conf.Environment.Run,
		}).Info("Container is already running...")
		os.Exit(0)
	}

	_, exists, err = ImageExists(client, image)
	if exists {
		log.WithFields(log.Fields{
			"exisits": exists,
			// "id":      elkContainer.ID,
			"env": config.Conf.Environment.Run,
		}).Infof("Image `%s` already pulled.", image)
	} else {
		log.WithFields(log.Fields{
			"exisits": exists,
			"env":     config.Conf.Environment.Run}).Infof("Pulling Image `%s`", image)

		assert(PullImage(client, image, "latest"))
	}
	createContConf := docker.Config{
		Image: image,
	}

	// portBindings := map[docker.Port][]docker.PortBinding{
	// 	"80/tcp":   {{HostIP: "0.0.0.0", HostPort: "80"}},
	// 	"9200/tcp": {{HostIP: "0.0.0.0", HostPort: "9200"}},
	// }

	createContHostConfig := docker.HostConfig{
		// Binds:           []string{"/var/run:/var/run:rw", "/sys:/sys:ro", "/var/lib/docker:/var/lib/docker:ro"},
		// PortBindings: portBindings,
		// PublishAllPorts: true,
		Privileged: false,
	}

	createContOps := docker.CreateContainerOptions{
		Name:       name,
		Config:     &createContConf,
		HostConfig: &createContHostConfig,
	}

	cont, err = client.CreateContainer(createContOps)
	if err != nil {
		log.WithFields(log.Fields{"env": config.Conf.Environment.Run}).Errorf("CreateContainer error = %s\n", err)
	}

	err = client.StartContainer(cont.ID, nil)
	if err != nil {
		log.WithFields(log.Fields{"env": config.Conf.Environment.Run}).Errorf("StartContainer error = %s\n", err)
	}

	if logs {
		LogContainer(cont)
	}

	return
}

// StartELK creates an ELK container from the image blacktop/elk
func StartELK(logs bool) (cont *docker.Container, err error) {

	assert(PingDockerClient(client))

	_, exists, err := ContainerExists(client, "elk")

	if exists {
		log.WithFields(log.Fields{
			"exisits": exists,
			// "id":      elkContainer.ID,
			"env": config.Conf.Environment.Run,
			"url": "http://" + ip,
		}).Info("ELK is already running...")
		os.Exit(0)
	}

	_, exists, err = ImageExists(client, "blacktop/elk")
	if exists {
		log.WithFields(log.Fields{
			"exisits": exists,
			// "id":      elkContainer.ID,
			"env": config.Conf.Environment.Run,
		}).Info("Image `blacktop/elk` already pulled.")
	} else {
		log.WithFields(log.Fields{
			"exisits": exists,
			"env":     config.Conf.Environment.Run}).Info("Pulling Image `blacktop/elk`")

		assert(PullImage(client, "blacktop/elk", "latest"))
	}
	createContConf := docker.Config{
		Image: "blacktop/elk",
	}

	portBindings := map[docker.Port][]docker.PortBinding{
		"80/tcp":   {{HostIP: "0.0.0.0", HostPort: "80"}},
		"9200/tcp": {{HostIP: "0.0.0.0", HostPort: "9200"}},
	}

	createContHostConfig := docker.HostConfig{
		// Binds:           []string{"/var/run:/var/run:rw", "/sys:/sys:ro", "/var/lib/docker:/var/lib/docker:ro"},
		PortBindings: portBindings,
		// PublishAllPorts: true,
		Privileged: false,
	}

	createContOps := docker.CreateContainerOptions{
		Name:       "elk",
		Config:     &createContConf,
		HostConfig: &createContHostConfig,
	}

	cont, err = client.CreateContainer(createContOps)
	if err != nil {
		log.WithFields(log.Fields{"env": config.Conf.Environment.Run}).Errorf("CreateContainer error = %s\n", err)
	}

	err = client.StartContainer(cont.ID, nil)
	if err != nil {
		log.WithFields(log.Fields{"env": config.Conf.Environment.Run}).Errorf("StartContainer error = %s\n", err)
	}

	if logs {
		LogContainer(cont)
	}

	return
}

// LogContainer tails container logs to terminal
func LogContainer(cont *docker.Container) {

	opts := docker.LogsOptions{
		Container:    cont.ID,
		OutputStream: os.Stdout,
		ErrorStream:  os.Stderr,
		Follow:       true,
		Stdout:       true,
		Stderr:       true,
		// Since:        0,
		Timestamps: false,
		// Tail:         false,
		RawTerminal: false, // Usually true when the container contains a TTY.
	}

	assert(client.Logs(opts))
}

func handleClientError() {
	log.WithFields(log.Fields{
		"env":      config.Conf.Environment.Run,
		"endpoint": endpoint,
	}).Error("Unable to connect to docker client")
	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("docker-machine"); err != nil {
			log.Infof("Please install docker-machine by running: \n\tbrew install docker-machine\n\tdocker-machine create -d virtualbox %s\n\teval $(docker-machine env %s)\n", config.Conf.Docker.Name, config.Conf.Docker.Name)
		} else {
			log.Infof("Please start and source the docker-machine env by running: \n\tdocker-machine start %s\n\teval $(docker-machine env %s)\n", config.Conf.Docker.Name, config.Conf.Docker.Name)
		}
	case "linux":
		log.Info("Please start the docker daemon.")
	case "windows":
		if _, err := exec.LookPath("docker-machine.exe"); err != nil {
			log.Info("Please install docker-machine - https://www.docker.com/docker-toolbox")
		} else {
			log.Infof("Please start and source the docker-machine env by running: \n\tdocker-machine start %s\n\teval $(docker-machine env %s)\n", config.Conf.Docker.Name, config.Conf.Docker.Name)
		}
	}
	// TODO Decide if I want to make docker machines or rely on use to create their own.
	// log.Info("Trying to create new docker-machine: ", "test")
	// MakeDockerMachine("test")
	os.Exit(2)
}