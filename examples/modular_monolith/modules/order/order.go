// Package order demonstrates a self-contained modular-monolith order domain.
package order

import (
	"net/http"
	"time"

	gpp "github.com/saifsilver/goplusplus"
)

// OrderModule represents the self-contained Order domain module.
type OrderModule struct{}

// New creates a new OrderModule instance.
func New() *OrderModule {
	return &OrderModule{}
}

// Name returns the identifier of the module.
func (m *OrderModule) Name() string {
	return "OrderModule"
}

// Register registers all Order domain endpoints onto the supplied RouterGroup.
func (m *OrderModule) Register(group *gpp.RouterGroup) {
	group.GET("/:id", m.getOrder)
	group.POST("/checkout", m.checkout)
}

func (m *OrderModule) getOrder(c *gpp.Context) error {
	id := c.Param("id")
	return c.JSON(http.StatusOK, gpp.H{
		"module":     m.Name(),
		"order_id":   id,
		"total":      149.99,
		"currency":   "USD",
		"status":     "SHIPPED",
		"created_at": time.Now().Add(-24 * time.Hour),
	})
}

func (m *OrderModule) checkout(c *gpp.Context) error {
	type checkoutReq struct {
		ItemID   string  `json:"item_id"`
		Quantity int     `json:"quantity"`
		Price    float64 `json:"price"`
	}

	var req checkoutReq
	if err := c.BindJSON(&req); err != nil {
		return gpp.NewHTTPError(http.StatusBadRequest, "Invalid checkout payload")
	}

	return c.JSON(http.StatusCreated, gpp.H{
		"module":   m.Name(),
		"order_id": "ord_88192",
		"message":  "Order placed successfully",
		"total":    float64(req.Quantity) * req.Price,
	})
}
