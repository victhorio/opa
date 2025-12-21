package core

type Response struct {
	Model    string
	Usage    Usage
	Messages []Message
}

type Usage struct {
	Input     int64
	Cached    int64
	Output    int64
	Reasoning int64
	Total     int64
	// unit here is thousandth of a millionth of a dollar
	// this means that a value of a billion equals 1 USD
	Cost int64
}

func (u *Usage) Inc(ou Usage) {
	u.Input += ou.Input
	u.Cached += ou.Cached
	u.Output += ou.Output
	u.Reasoning += ou.Reasoning
	u.Total += ou.Total
	u.Cost += ou.Cost
}
