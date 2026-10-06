package client

// Public deterministic test vector, never a publisher identity. It proves that
// the system helper supports SSH signature verification before checking a release.
const sshCapabilitySigner = "AAAAC3NzaC1lZDI1NTE5AAAAIAOhB7/zzhC+HXDdGOdLwJln5NYwm6UNXx3chmQSVTG4"
const sshCapabilityMessage = "SECONDED ssh-keygen capability fixture v1\n"
const sshCapabilitySignature = `-----BEGIN SSH SIGNATURE-----
U1NIU0lHAAAAAQAAADMAAAALc3NoLWVkMjU1MTkAAAAgA6EHv/POEL4dcN0Y50vAmWfk1j
CbpQ1fHdyGZBJVMbgAAAAQc2Vjb25kZWQtcmVsZWFzZQAAAAAAAAAGc2hhNTEyAAAAUwAA
AAtzc2gtZWQyNTUxOQAAAEBK4fadCjYDxJKdPLwp11qkrxxvSK98dnRFgEk0tMNc+Hc2XX
vt7jrYFKeQ7XoErKvq+1/+MXUoT+4pTdGd7WwO
-----END SSH SIGNATURE-----
`
