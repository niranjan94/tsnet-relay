[![DeepSource](https://app.deepsource.com/gh/niranjan94/tsnet-relay.svg/?label=active+issues&show_trend=true&token=Do84hQ3_bWTShmuaJu5RikAE)](https://app.deepsource.com/gh/niranjan94/tsnet-relay/)

## Tailscale Relay

> Work in progress. Expect breaking changes.

Easily create relays b/w services running on tailscale

### Usage

```
Usage of ./tsnet-relay:
  -advertise-tags string
        Tags to use for the server
  -config string
        Path to the configuration file (default "config.json")
  -ephemeral
        Use an ephemeral hostname
  -hostname string
        Hostname to use for the server
  -idle-timeout int
        Exit after specified number of seconds with no incoming connections (0 to disable)
  -state string
        State store to use for the server (default "mem:")
  -verbose
        Enable verbose logging
```


### Configuration

The configuration file is a JSON file with the following format:

```json
{
  "tunnels": [
    {
      "enabled": true,
      "name": "expose-remote-locally",
      "source": "tcp://:3000",
      "destination": "tcp+tailnet://fake-server.fake-network.ts.net:2746"
    },
    {
      "enabled": true,
      "name": "expose-local-on-tsnet",
      "source": "tcp+tailnet://:3001",
      "destination": "tcp://127.0.0.1:3000"
    }
  ]
}
```