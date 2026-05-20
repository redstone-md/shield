# updater

A small utility to update tg-spam samples from remote git repository

## Usage

The utility is designed to be run either as a docker container or as a standalone script or as a part of a cron job. It detects the environment and acts accordingly. For example, if it is run as a docker container, it will update the samples every minute. However, if it runs as a part of a cron job, it will update the samples once per run, only if the remote repository has changed since the last update (or if the local repository is missing).

### Docker

The docker image is available from [GitHub Packages](https://github.com/redstone-md/shield/pkgs/container/shield-updater). The following command arguments are supported:

- first argument is a git repository url (required)
- second argument is a path to the local repository (optional, default is `./samples`)

**Example of running the utility as a docker container:**

```bash
docker run -d --name shield-updater -v $(pwd)/tg-spam-samples:/samples ghcr.io/redstone-md/shield-updater https://github.com/radio-t/tg-spam-samples.git /samples
```
The command above will run the updater as a docker container and mount the local directory `./tg-spam-samples` to the container's `/samples` directory. The updater will clone the remote repository to the local directory and then update it every minute.

**Example of running the utility from the docker-compose:**

```yaml
services:
  shield-updater:
    image: ghcr.io/redstone-md/shield-updater:latest
    restart: always
    user: "1000:1000" # run with the same user as the host machine to avoid permission issues
    command: ["https://github.com/radio-t/tg-spam-samples.git", "/samples"]
    volumes:
      - ./tg-spam-samples:/samples
```

**permission issues**

If the updater is run as a Docker container, there may be a discrepancy in file ownership between the host and the container. Set `user: "1000:1000"` in Compose or pass `--user $(id -u):$(id -g)` to `docker run` when the mounted samples directory should be owned by the host user. It's also recommended to create the samples directory on the host before running the container.
