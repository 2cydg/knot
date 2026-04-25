package sftp

type PathKind int

const (
	PathNone PathKind = iota
	PathLocalFile
	PathLocalDir
	PathLocalAny
	PathLocalPattern
	PathRemoteFile
	PathRemoteDir
	PathRemoteAny
	PathRemotePattern
)

type ArgSpec struct {
	Kind     PathKind
	Optional bool
}

type CommandSpec struct {
	Name    string
	Aliases []string
	Args    []ArgSpec
}

var replCommandSpecs = []*CommandSpec{
	{Name: "help", Aliases: []string{"?"}},
	{Name: "exit", Aliases: []string{"quit", "bye"}},
	{Name: "ls", Args: []ArgSpec{{Kind: PathRemoteAny, Optional: true}}},
	{Name: "pwd"},
	{Name: "cd", Args: []ArgSpec{{Kind: PathRemoteDir}}},
	{Name: "get", Args: []ArgSpec{
		{Kind: PathRemoteAny},
		{Kind: PathLocalAny, Optional: true},
	}},
	{Name: "put", Args: []ArgSpec{
		{Kind: PathLocalAny},
		{Kind: PathRemoteAny, Optional: true},
	}},
	{Name: "mget", Args: []ArgSpec{
		{Kind: PathRemotePattern},
		{Kind: PathLocalDir, Optional: true},
	}},
	{Name: "mput", Args: []ArgSpec{
		{Kind: PathLocalPattern},
		{Kind: PathRemoteDir, Optional: true},
	}},
	{Name: "rm", Args: []ArgSpec{{Kind: PathRemoteAny}}},
	{Name: "mkdir", Args: []ArgSpec{{Kind: PathRemoteDir}}},
	{Name: "rmdir", Args: []ArgSpec{{Kind: PathRemoteDir}}},
}

var replCommandIndex = buildCommandIndex(replCommandSpecs)

func buildCommandIndex(specs []*CommandSpec) map[string]*CommandSpec {
	index := make(map[string]*CommandSpec, len(specs)*2)
	for _, spec := range specs {
		index[spec.Name] = spec
		for _, alias := range spec.Aliases {
			index[alias] = spec
		}
	}
	return index
}

func lookupCommandSpec(name string) *CommandSpec {
	return replCommandIndex[name]
}
