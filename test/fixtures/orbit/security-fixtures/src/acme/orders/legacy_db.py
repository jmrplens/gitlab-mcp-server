"""Legacy database accessors — these methods contain SQL injection
vulnerabilities on purpose so SAST finds them."""

import sqlite3
from typing import Any, Optional


def _connect() -> sqlite3.Connection:
    return sqlite3.connect(":memory:")


def fetch_order_unsafe(order_id: str) -> Optional[Any]:
    """Look up an order by id via raw f-string interpolation — DO NOT
    copy this pattern into real code."""
    conn = _connect()
    cur = conn.cursor()
    # INTENTIONAL SQL INJECTION VECTOR for fixture SAST detections.
    cur.execute(f"SELECT * FROM orders WHERE id = '{order_id}'")
    row = cur.fetchone()
    conn.close()
    return row


def save_payment_unsafe(order_id: str, amount_cents: int, note: str) -> None:
    """Persist a payment record using string concatenation — DO NOT
    copy this pattern into real code."""
    conn = _connect()
    cur = conn.cursor()
    # INTENTIONAL SQL INJECTION VECTOR for fixture SAST detections.
    cur.execute("INSERT INTO payments (order_id, amount_cents, note) VALUES ('" + order_id + "', " + str(amount_cents) + ", '" + note + "')")
    conn.commit()
    conn.close()
