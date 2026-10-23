# Okube

A lightweight, network-aware container orchestrator inspired by Kubernetes, designed for on-premise server architectures.

## Overview

Okube is a container orchestration system that manages containerized workloads across a cluster of nodes. Built with Go and Docker, it provides a simplified alternative to Kubernetes for on-premise deployments, offering task scheduling, resource management, and automated container lifecycle management.

## Features

- **Container Orchestration**: Manage containerized applications using Docker
- **Task Scheduling**: Intelligent task placement across worker nodes
- **Resource Management**: Track and allocate CPU, memory, and disk resources
- **Worker-Manager Architecture**: Distributed system with clear separation of concerns
- **CLI Interface**: User-friendly command-line tool for cluster management
- **State Management**: Track task states (Pending, Scheduled, Running, Completed, Failed)
- **Network Awareness**: Designed with network topology in mind for on-premise environments

## Architecture

### Components

- **Manager**: Orchestrates task distribution, maintains cluster state, and makes scheduling decisions
- **Worker**: Executes tasks on individual nodes, reports resource usage and task status
- **Scheduler**: Selects optimal nodes for task placement based on resource availability
- **Node**: Represents physical or virtual machines in the cluster
- **Task**: Unit of work representing a containerized application

### Project Structure

```
okube/
├── cmd/                   # Cobra CLI commands (single binary)
│   ├── root.go            # Root command, --manager flag
│   ├── manager.go         # `okube manager` — start a manager node
│   ├── worker.go          # `okube worker` — start a worker node
│   └── cli.go             # `okube run/stop/status/nodes/deploy/apps/delete`
├── pkg/
│   ├── manager/           # Manager component
│   ├── worker/            # Worker component
│   ├── scheduler/         # Scheduling logic (roundrobin, epvm)
│   ├── manifest/          # Multi-service YAML manifest parsing
│   ├── appgroup/          # Service dependency graph
│   ├── task/              # Task definitions and Docker operations
│   ├── store/             # etcd-backed cluster state
│   └── node/               # Node representation
├── main.go                # Application entry point
└── go.mod                 # Go module dependencies
```

## Installation

### Prerequisites

- Go 1.25.3 or higher
- Docker installed and running
- etcd (used as the manager's cluster store)
- Linux operating system (recommended)

### Build from Source

```bash
# Clone the repository
git clone https://github.com/aditip149209/okube.git
cd okube

# Install dependencies
go mod download

# Build the binary (includes the manager, worker, and CLI commands)
go build -o okube .
```

## Usage

### Starting a Manager

An etcd instance must be reachable first (defaults to `localhost:2379`).

```bash
./okube manager --host 0.0.0.0 --port 5556 --etcd-endpoints localhost:2379 --scheduler epvm
```

Key manager flags:
- `--scheduler`: `roundrobin` or `epvm` (network/energy-aware)
- `--network-weight`, `--resource-weight`, `--energy-weight`: fused scoring weights (epvm scheduler)
- `--carbon-intensity-file`: path to a JSON file mapping worker ID/address to carbon intensity (see [carbon_intensity.json](carbon_intensity.json))
- `--disable-network-score`, `--disable-energy-score`: baseline evaluation runs

### Starting a Worker

```bash
./okube worker --port 5557 --name worker-1 --manager-host localhost --manager-port 5556
```

### Using the CLI

All CLI commands accept `-m/--manager` (comma-separated `host:port`, defaults to `localhost:5556`).

#### Submit a Task

```bash
./okube run -f task1.json
```

#### Deploy a Multi-Service Application

```bash
./okube deploy -f manifest.yaml
./okube apps
./okube delete demo-app
```

#### Inspect the Cluster

```bash
./okube status   # manager role/leader + all tasks
./okube nodes     # registered worker nodes
./okube stop <task-id>
```

## API Reference

### Task States

Tasks progress through the following states:

- `Pending`: Task created but not yet scheduled
- `Scheduled`: Task assigned to a worker node
- `Running`: Task container is active
- `Completed`: Task finished successfully
- `Failed`: Task encountered an error

### Task Configuration

```go
type Task struct {
    ID            uuid.UUID
    Name          string
    State         State
    Image         string
    Memory        int
    Disk          int
    ExposedPorts  nat.PortSet
    PortBindings  map[string]string
    RestartPolicy string
    StartTime     time.Time
    EndTime       time.Time
}
```

### Node Configuration

```go
type Node struct {
    Name            string
    Ip              string
    Cores           int
    Memory          int
    MemoryAllocated int
    Disk            int
    DiskAllocated   int
    Role            string
    TaskCount       int
}
```

## Development

### Dependencies

Key dependencies include:
- **Docker SDK**: `github.com/docker/docker/client`
- **Cobra**: CLI framework
- **UUID**: Unique identifier generation
- **Go Collections**: Queue implementation

### Running Tests

```bash
go test ./...
```

### Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## Roadmap

- [ ] HTTP API server for remote management
- [ ] Advanced scheduling algorithms
- [ ] Health checks and auto-recovery
- [ ] Resource quotas and limits
- [ ] Multi-manager high availability
- [ ] Service discovery and networking
- [ ] Persistent volume support
- [ ] Configuration file support (YAML/JSON)
- [ ] Monitoring and metrics collection
- [ ] Web-based dashboard

## Technical Details

### Resource Management

Each node tracks:
- CPU cores (available and allocated)
- Memory (total and allocated)
- Disk space (total and allocated)
- Running task count

### Docker Integration

Okube uses the Docker SDK to:
- Pull container images
- Create and start containers
- Configure resource limits (CPU, memory)
- Set up port bindings and environment variables
- Manage container lifecycle (start, stop, remove)

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Author

**Aditi P** - [aditip149209](https://github.com/aditip149209)

## Acknowledgments

- Inspired by Kubernetes architecture
- Built as a learning project for distributed systems and container orchestration
- Uses the Docker Engine API for container management

## Support

For issues, questions, or contributions, please open an issue on the [GitHub repository](https://github.com/aditip149209/okube).

---

**Note**: This is an educational project and is not recommended for production use. For production container orchestration, consider using Kubernetes, Docker Swarm, or other mature platforms.


<!-- 
todo: cloud provider interface, multiple managers using raft, network aware capabilities, custom network aware scheduler plugin for kubernetes to be donated -->

