package instrumentlang

const (
	// HACK odiglet restart will cause the mounted directory to be lost
	// Use the parent directory to mount it to avoid loss.
	commonMountPath = "/var/odigos"

	dotnetMountPath = "/var/odigos/dotnet"
	javaMountPath   = "/var/odigos/java"
	pythonMountPath = "/var/odigos/python"
	nodeMountPath   = "/var/odigos/nodejs"
)
