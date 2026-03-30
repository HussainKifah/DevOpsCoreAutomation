package nocpass

// Fixed NOC local accounts on every managed device (same rotating password on both).
const (
	UserFiberx   = "fiberx"
	UserReadOnly = "readOnly"
)

// AccountSummary describes the two accounts for API/UI.
var AccountSummary = []struct {
	Username string
	Hint     string
}{
	{UserFiberx, "privilege 15 (full)"},
	{UserReadOnly, "privilege 13 (read-only)"},
}
