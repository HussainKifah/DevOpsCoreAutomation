package nocpass

// Fixed NOC local accounts on every managed device.
const (
	UserFiberx  = "fiberx"
	UserSupport = "support"
	UserDev     = "dev"
)

// AccountSummary describes the managed accounts for API/UI.
var AccountSummary = []struct {
	Username string
	Hint     string
}{
	{UserFiberx, "privilege 15 (full)"},
	{UserSupport, "write-capable support access"},
}
