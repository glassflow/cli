<div align="center">
  <img src="https://learn.glassflow.dev/~gitbook/image?url=https:%2F%2F3630921082-files.gitbook.io%2F%7E%2Ffiles%2Fv0%2Fb%2Fgitbook-x-prod.appspot.com%2Fo%2Fspaces%252FpRyi93X0Jn9wrh2Z4Ffm%252Flogo%252Fj4ZLY66JC4CCI0kp4Tcl%252FBlue.png%3Falt=media%26token=824ab2c7-e9a7-4b53-bd9a-375650951fc1&width=128&dpr=2&quality=100&sign=312af88abf1a93b897726483f4d86c2733192ab70b94b68ba438f6c85caf7e1a" /><br /><br />
</div>
<p align="center">
<a href="https://join.slack.com/t/glassflowhub/shared_invite/zt-2g3s6nhci-bb8cXP9g9jAQ942gHP5tqg">
        <img src="https://img.shields.io/badge/slack-join-community?logo=slack&amp;logoColor=white&amp;style=flat"
            alt="Chat on Slack"></a>

# GlassFlow CLI

The GlassFlow Command Line Interface (CLI) simplifies the process of creating, managing, and monitoring your data pipelines on the GlassFlow platform. It's built for developers, data engineers, and IT professionals who prefer working within a command-line environment to automate tasks and streamline their workflow. The CLI directly interacts with the [GlassFlow API](https://api.glassflow.xyz/v1/docs) for pipeline management.

## Features

- **Pipeline Management**: Create, update, delete, and list organizations, spaces, and data pipelines.
- **Real-Time Data Processing**: Send data to and consume data from your pipelines.
- **Monitoring and Logs**: Access real-time logs and monitor the performance of your pipelines.
- **Secure Authentication**: Manage your GlassFlow credentials securely.

## Installation

The CLI is available for **macOS, Linux, and Windows** OS, and can be installed using standard package managers like [Homebrew](https://brew.sh/).


### Install using Homebrew

Install the GlassFlow CLI using the Homebrew:

```bash
brew tap glassflow/tap
brew install glassflow
```

This installs the GlassFlow command globally so you can run `glassflow`commands from any directory.

### Install from the release package

For **Linux** based systems, we support installation by downloading the release version via GitHub:

```bash
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/glassflow/cli/master/install.sh)"
```

To install the CLI on **Windows** OS, follow the [guide in the documentation](https://learn.glassflow.dev/docs/get-started/glassflow-cli#install-on-windows-powershell).

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
$ glassflow --help

Usage: glassflow [OPTIONS] COMMAND [arg...]

GlassFlow - Python-based data streaming pipelines within minutes.

Options:
      --version   Show the version and exit
  -v, --verbose   Verbose output

Commands:
  signup          Create new account
  login           Log in to GlassFlow
  profile         Get profile data
  logout          Log out from GlassFlow
  organization    Manage organizations
  space           Manage spaces
  pipeline        Manage pipelines
  version         Show the version
```

You can also see available subcommands for a given command by running `glassflow command --help`. For example:

```bash
$ glassflow pipeline --help

Usage: glassflow pipeline [OPTIONS] COMMAND [arg...]

Manage pipelines

Options:

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
