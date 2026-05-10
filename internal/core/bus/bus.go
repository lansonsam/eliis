package bus

// Bus is the internal pub/sub facade used for cross-module notifications (M0 placeholder).
type Bus struct{}

// New returns a bus instance. Wiring is added in later milestones.
func New() *Bus {
	return &Bus{}
}
