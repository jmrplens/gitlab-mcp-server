"""Entry-point shim so the package can be invoked as `python -m acme.orders`."""

from acme.orders.cli import main

if __name__ == "__main__":
    raise SystemExit(main())
