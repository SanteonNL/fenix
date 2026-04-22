package body

type ErrorResponse struct {
	Code        string  `json:"code"`
	Description string  `json:"description"`
	Value       *string `json:"value,omitempty"`
	Module      *string `json:"module,omitempty"`
	Line        *any    `json:"line,omitempty"`
	Column      *any    `json:"column,omitempty"`
}
