package hub

type ShellRunner interface {
	Run(utilityName string, arguments ...string) error
}
