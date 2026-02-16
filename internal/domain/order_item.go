package domain

type OrderItem struct {
	ID        uint
	OrderID   uint
	ProductID int
	Quantity  int
	Price     float64
}
