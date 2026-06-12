"""acme.orders — tiny order-processing package used as Orbit KG fixture.

The package is split into four modules so the Knowledge Graph indexer can
extract one `Definition` per top-level symbol plus a handful of cross-file
`ImportedSymbol` edges.
"""

from acme.orders.models import Order, OrderItem, OrderStatus
from acme.orders.pricing import compute_subtotal, apply_discount, compute_total
from acme.orders.fulfillment import reserve_stock, schedule_pickup, mark_shipped

__all__ = [
    "Order",
    "OrderItem",
    "OrderStatus",
    "compute_subtotal",
    "apply_discount",
    "compute_total",
    "reserve_stock",
    "schedule_pickup",
    "mark_shipped",
]

__version__ = "0.1.0"
