# Contributing to vJailbreak

Thank you for your interest in contributing to vJailbreak! This document provides guidelines and instructions for contributing to the project.

## Code of Conduct

By participating in this project, you agree to abide by our code of conduct. Please be respectful and considerate of others when participating.

## Getting Started

1. Fork the repository on GitHub
2. Clone your fork to your local machine
3. Create a branch for your changes
4. Make your changes
5. Push your branch to your fork
6. Submit a pull request

## Development Setup

### Prerequisites

The same prerequisites apply for development as for using vJailbreak:

- VMware vCenter with appropriate permissions
- OpenStack-compliant cloud target 
- Network connectivity between environments
- Development machine with Go 1.20+ installed

### Local Development

1. Clone the repository:
   ```bash
   git clone https://github.com/platform9/vjailbreak.git
   cd vjailbreak
   ```

2. Install dependencies:
   ```bash
   go mod download
   ```

3. Build the project:
   ```bash
   make build
   ```

4. Run tests:
   ```bash
   make test
   ```

## Pull Request Guidelines

1. **Branch naming**: Use descriptive branch names (e.g., `feature/add-migration-validation`, `fix/vmware-connection-issue`)
2. **Commit messages**: Write clear, concise commit messages describing the changes
3. **Documentation**: Update documentation to reflect your changes
4. **Tests**: Add tests for new features and ensure all tests pass
5. **Code style**: Follow the project's code style and formatting guidelines

## Code Review Process

1. Maintainers will review your pull request
2. Address any feedback or requested changes
3. Once approved, a maintainer will merge your pull request

## Issue Reporting

If you find a bug or have a feature request:

1. Check if the issue already exists in the GitHub issue tracker
2. If not, create a new issue with a descriptive title and detailed description
3. Include steps to reproduce for bugs
4. Add relevant labels

## License

By contributing to vJailbreak, you agree that your contributions will be licensed under the project's [Business Source License 1.1](LICENSE).

Thank you for contributing to vJailbreak!
