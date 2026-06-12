"""acme.orders — fixture package with intentional vulnerable patterns.

The code in this module is intentionally insecure. It exists so the
GitLab SAST, Secret Detection, and Dependency Scanning analyzers emit
real Finding and Vulnerability records that the Orbit knowledge graph
indexes under the security domain.
"""

# These look like real secrets to a regex-based detector. They are
# obviously fake values; do NOT use them anywhere outside this fixture.
AWS_ACCESS_KEY_ID = "AKIAFAKEFIXTURE0000001"
AWS_SECRET_ACCESS_KEY = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
GITHUB_TOKEN = "ghp_FIXTURE0000000000000000000000000001"

from acme.orders.legacy_db import fetch_order_unsafe, save_payment_unsafe
from acme.orders.auth import verify_password_weak, build_query_naive

__all__ = [
    "fetch_order_unsafe",
    "save_payment_unsafe",
    "verify_password_weak",
    "build_query_naive",
]
