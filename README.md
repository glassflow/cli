# GlassFlow CLI

The GlassFlow CLI simplifies the process of creating, managing, and monitoring your data pipelines on the GlassFlow platform. It's built for developers, data engineers, and IT professionals who prefer working within a command-line environment to automate tasks and streamline their workflow. The CLI directly interacts with the [GlassFlow API](https://api.glassflow.xyz/v1/docs) for pipeline management.

## Features

- **Pipeline Management**: Create, update, delete, and list organizations, spaces and data pipelines.
- **Real-Time Data Processing**: Send data to and consume data from your pipelines.
- **Monitoring and Logs**: Access real-time logs and monitor the performance of your pipelines.
- **Secure Authentication**: Manage your GlassFlow credentials securely.

## Installation

The CLI is available for **macOS, and Linux**, and can be installed using standard package managers like [Homebrew](https://brew.sh/).

> If you are a Windows user, we highly recommend leveraging [Windows Subsystem for Linux (WSL)](https://learn.microsoft.com/en-us/windows/wsl/install)

### Install using Homebrew

Install the GlassFlow CLI using the Homebrew:

```bash
brew tap glassflow/tap
brew install glassflow
```

This installs the GlassFlow command globally so you can run `glassflow`commands from any directory.

### Install from source

For Linux-based systems, we support installation by downloading the release version via GitHub:

```bash
$ /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/glassflow/install/HEAD/install.sh)" 
```

## Sign Up

After installing the CLI, simply open your terminal and run the following command to create an account on GlassFlow:

```bash
glassflow signup
```

Upon executing this command, you'll be prompted to choose your preferred method of signup—either using your Google account or via GitHub authentication.

## Getting Help

Open your terminal and type `glassflow --help` to see a list of available commands and options. This command provides a quick reference to the capabilities of the CLI, including creating, removing, and managing your account, organization, space, and pipelines.

The general form of the CLI usage is:

```bash
glassflow --helpglassflow command [subcommand] [options]
```

```bash
$glassflow --help

GlassFlow - Python-based data streaming pipelines within minutes.

Options:
      --version          Show the version and exit
  -d, --debug            Debug output
  -v, --verbose          Verbose output
  -t, --client-timeout   HTTP client timeout in seconds (default 30)
  -r, --remote           Remote address of GlassFlow API server

Commands:
  signup                 Create new account
  login                  Log in to GlassFlow
  profile                Get profile data
  logout                 Log out from GlassFlow
  organization           Manage organizations
  space                  Manage spaces
  pipeline               Manage pipelines
  version                Show the version
```

You can also see available subcommands for a given command by running `glassflow command --help`. For example:

```bash
Usage: glassflow pipeline [OPTIONS] COMMAND [arg...]

Manage pipelines

Options:
  -o, --org         Organization ID (default 00000000-0000-0000-0000-000000000000)

Commands:
  list              Get pipelines
  create            Create pipeline
  get               Get pipeline
  delete            Delete pipeline
  update-function   Update function
  logs              Get function logs
  tokens            Get list of access tokens
  token-generate    Generate new access token
  token-rename      Rename access token
  token-revoke      Revoke access token
```

## Examples

Visit the [GlassFlow examples](https://github.com/glassflow/glassflow-examples) repository to explore how to build new pipelines using CLI.

## User Guides

For more detailed information on how to use the GlassFlow CLI, please refer to the [GlassFlow Documentation](https://learn.glassflow.dev/). The documentation provides comprehensive guides, tutorials, and examples to help you get started with GlassFlow CLI. 
