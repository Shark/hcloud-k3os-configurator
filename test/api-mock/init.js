(async () => {
    const mockServerClient = require("mockserver-client").mockServerClient("hcloud", 80)
    const fs = require("fs")
    const assert = require("assert")

    const args = process.argv.slice(2)
    const instanceID = args[0]
    assert(typeof instanceID !== "undefined" && instanceID.length > 0)

    await mockServerClient.reset()
    const fixtures = [
        mockServerClient.mockAnyResponse({
            "httpRequest": {
                "method": "GET",
                "headers": {
                   "Host": ["169.254.169.254"]
                },
                "path": "/latest/user-data"
            },
            "httpResponse": {
                "body": fs.readFileSync("fixtures/latest/user-data.yml", "utf8")
            }
        }),
        mockServerClient.mockAnyResponse({
            "httpRequest": {
                "method": "GET",
                "headers": {
                    "Host": ["169.254.169.254"]
                },
                "path": "/hetzner/v1/metadata/instance-id"
            },
            "httpResponse": {
                "body": instanceID
            }
        }),
        mockServerClient.mockAnyResponse({
            "httpRequest": {
                "method": "GET",
                "headers": {
                    "Host": ["api.hetzner.cloud"],
                    "Authorization": ["Bearer hcloudtoken"]
                },
                "path": "/v1/networks/50343"
            },
            "httpResponse": {
                "headers": {
                    "Content-Type": ["application/json"]
                },
                "body": fs.readFileSync("fixtures/v1/networks/50343.json", "utf8")
            }
        }),
        mockServerClient.mockAnyResponse({
            "httpRequest": {
                "method": "GET",
                "headers": {
                    "Host": ["api.hetzner.cloud"],
                    "Authorization": ["Bearer hcloudtoken"]
                },
                "path": "/v1/floating_ips"
            },
            "httpResponse": {
                "headers": {
                    "Content-Type": ["application/json"]
                },
                "body": fs.readFileSync("fixtures/v1/floating_ips.json", "utf8")
            }
        }),
        mockServerClient.mockAnyResponse({
            "httpRequest": {
                "method": "GET",
                "headers": {
                    "Host": ["api.hetzner.cloud"],
                    "Authorization": ["Bearer hcloudtoken"]
                },
                "path": "/v1/servers"
            },
            "httpResponse": {
                "headers": {
                    "Content-Type": ["application/json"]
                },
                "body": fs.readFileSync("fixtures/v1/_servers.json", "utf8")
            }
        }),
        mockServerClient.mockAnyResponse({
            "httpRequest": {
                "method": "GET",
                "headers": {
                    "Host": ["api.hetzner.cloud"],
                    "Authorization": ["Bearer hcloudtoken"]
                },
                "path": "/v1/servers/4406144"
            },
            "httpResponse": {
                "headers": {
                    "Content-Type": ["application/json"]
                },
                "body": fs.readFileSync("fixtures/v1/servers/4406144.json", "utf8")
            }
        }),
        mockServerClient.mockAnyResponse({
            "httpRequest": {
                "method": "GET",
                "headers": {
                    "Host": ["api.hetzner.cloud"],
                    "Authorization": ["Bearer hcloudtoken"]
                },
                "path": "/v1/servers/4406228"
            },
            "httpResponse": {
                "headers": {
                    "Content-Type": ["application/json"]
                },
                "body": fs.readFileSync("fixtures/v1/servers/4406228.json", "utf8")
            }
        })
    ]
    return Promise.all(fixtures)
})().catch(e => {
    console.log(`Error: ${e}`)
});