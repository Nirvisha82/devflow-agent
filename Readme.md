# Automate GitHub Issues - App

This app aims to automate GitHub Issues Lifecyle.

## Setup

### Pre-requisites

#### Golang

- Install Go.
- Open terminal in root directory and run

```bash
go get
```

#### `smee`

- To install, run:

```bash
npm install -g smee-client
```

### Export Environment Variables
>
> If you have the private key file, copy it to the root directory.

1. Generate a private key

- Head over to [Devflow Agent Settings](url=https://github.com/settings/apps/devflow-agent)
- Scroll to the `Private Keys` section.
- Click on  `Generate a private Key`.
- This action will download a private key and you can then copy it to the root directory.
- Name of the key would be similar to `devflow-agent.2025-mm-dd.private-key.pem`
- Copy the path (preferrably relative) `.env` and paste it :

```bash
export GITHUB_APP_PRIVATE_KEY_PATH=devflow-agent.2025-mm-dd.private-key.pem
```

2. Create a smee link.(or use the one present)

- Go to [smee.io](url=https://smee.io)
- Click on `Start a new channel`
- Copy the link provide and paste it in .env:

```bash
export WEBHOOK_PROXY_URL=https://smee.io/<your-generate-url-here>
```

- Add this link to `Webhook` section on [Devflow Agent Settings](url=https://github.com/settings/apps/devflow-agent)
- Change secret if needed, or else, use the existing one - `development`

3. Export the variables.

- Copy and paste the content from `.env` into a terminal opened in the root directory.

## Run

1. Start the App server.

```bash
go run main.go
```

2. Configure the events stream.

```bash
smee --url WEBHOOK_PROXY_URL --path / --port <port-id>  # note the port id from prev. step.
smee --url https://smee.io/Dtgyfv0N4x0BOYkG --path / --port 8000 
```

---
The app now listens to events sent by GitHub from connected repositories.

Todos:
Create a branch and raise a PR - Once the issue is recieved, on parsing the description, the repository must be cloned and then a branch must be created with issues title or id or number or any xyz naming convention.
Then we must be able to accesss a particular file `devflow-config`, `CODEOWNERS` etc.
