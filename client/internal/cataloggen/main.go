// Command cataloggen exports the actual MCP tool schemas without loading a profile.
package main

import (
 "encoding/json"
 "os"
 client "seconded.local/client"
)

func main() {
 if err := json.NewEncoder(os.Stdout).Encode(client.Tools()); err != nil { panic(err) }
}
