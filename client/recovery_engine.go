package client

// RecoveryEngine configures the same pinned, read-only evidence used by an
// installed MCP engine, without loading a wallet or signing credential.
func RecoveryEngine(files *Files) (*Engine, error) {
	api, err := ReleaseAPI()
	if err != nil {
		debugRecovery("recovery_configuration_failed", err)
		return nil, err
	}
	chain, err := ReleaseEvidence()
	if err != nil {
		debugRecovery("recovery_configuration_failed", err)
		return nil, err
	}
	return &Engine{Files: files, API: api, Chain: chain}, nil
}
