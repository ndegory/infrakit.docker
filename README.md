# InfraKit.Docker

[InfraKit](https://github.com/docker/infrakit) plugins for creating and managing Docker containers.

## Instance plugin

An InfraKit instance plugin is provided, which creates Docker containers.

### Building and running

To build the Docker instance plugin, run `make binaries`.  The plugin binary will be located at
`./build/infrakit-instance-docker`.

### Example

To continue with an example, we will use the [default](https://github.com/docker/infrakit/tree/master/cmd/group) Group
plugin:
```console
$ build/infrakit-group-default
INFO[0000] Starting discovery
INFO[0000] Starting plugin
INFO[0000] Starting
INFO[0000] Listening on: unix:///run/infrakit/plugins/group.sock
INFO[0000] listener protocol= unix addr= /run/infrakit/plugins/group.sock err= <nil>
```

and the [Vanilla](https://github.com/docker/infrakit/tree/master/pkg/example/flavor/vanilla) Flavor plugin:.
```console
$ build/infrakit-flavor-vanilla
INFO[0000] Starting plugin
INFO[0000] Listening on: unix:///run/infrakit/plugins/flavor-vanilla.sock
INFO[0000] listener protocol= unix addr= /run/infrakit/plugins/flavor-vanilla.sock err= <nil>
```

We will use a basic configuration that creates a single instance:
```console
$ cat << EOF > aws-vanilla.json
{
  "ID": "docker-example",
  "Properties": {
    "Allocation": {
      "Size": 1
    },
    "Instance": {
      "Plugin": "instance-docker",
      "Properties": {
        "Config": {
          "Image": "alpine:3.5",
        },
        "Tags": {
          "Name": "infrakit-example"
        }
      }
    },
    "Flavor": {
      "Plugin": "flavor-vanilla",
      "Properties": {
        "Init": [
          "sh -c \"echo 'Hello, World!' > /hello\""
        ]
      }
    }
  }
}
EOF
```

For the structure of `Config`, please refer to [the Docker SDK for Go](https://github.com/docker/docker/blob/master/api/types/container/config.go).
You can also define a `HostConfig` element, [as described here](https://github.com/docker/docker/blob/master/api/types/container/host_config.go).

Finally, instruct the Group plugin to start watching the group:
```console
$ build/infrakit group watch docker-vanilla.json
watching docker-example
```

In the console running the Group plugin, we will see input like the following:
```
INFO[1208] Watching group 'aws-example'
INFO[1219] Adding 1 instances to group to reach desired 1
INFO[1219] Created instance i-ba0412a2 with tags map[infrakit.config_sha:dUBtWGmkptbGg29ecBgv1VJYzys= infrakit.group:aws-example]
```

Additionally, the CLI will report the newly-created instance:
```console
$ build/infrakit group inspect docker-example
ID                             	LOGICAL                        	TAGS
90e6f3de4918                   	elusive_leaky                  	Name=infrakit-example,infrakit.config_sha=dUBtWGmkptbGg29ecBgv1VJYzys=,infrakit.group=docker-example
```

Retrieve the name of the container and connect to it with an exec

```console
$ docker exec -ti elusive_leaky cat /hello
Hello, World!
```

### Plugin properties

The plugin expects properties in the following format:
```json
{
  "Tags": {
  },
  "Config": {
  }
}
```

The `Tags` property is a string-string mapping of labels to apply to all Docker containers that are created.
`Config` follows the structure of the type by the same name in the
[Docker go SDK](https://github.com/docker/docker/blob/master/api/types/container/config.go).
