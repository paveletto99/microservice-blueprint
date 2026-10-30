#!/usr/bin/env python3
"""Exercise real order persistence through Service A without handling bearer secrets."""
import json
import os
import sys
import urllib.error
import urllib.request
import uuid

BASE = os.environ.get("ORDERS_CALL_URL", "http://127.0.0.1:18081/call/orders").rstrip("/")


def call(method, url, item=None, expected=200):
    data = json.dumps({"item": item}).encode() if item is not None else None
    request = urllib.request.Request(url, data=data, method=method)
    if data is not None:
        request.add_header("Content-Type", "application/json")
    with urllib.request.urlopen(request, timeout=10) as response:
        if response.status != expected:
            raise RuntimeError("unexpected CRUD status")
        body = response.read(262144)
        return json.loads(body) if body else None


def run():
    item = "crud-smoke-" + str(uuid.uuid4())
    created = call("POST", BASE, item, expected=201)["order"]
    order_id = created["id"]
    deleted = False
    try:
        if call("GET", BASE + "/" + order_id)["order"]["item"] != item:
            raise RuntimeError("created data did not persist")
        for _ in range(2):
            if call("PUT", BASE + "/" + order_id, "updated-smoke")["order"]["item"] != "updated-smoke":
                raise RuntimeError("same-value update failed")
        call("GET", BASE + "?limit=1")
        call("DELETE", BASE + "/" + order_id, expected=204)
        deleted = True
        try:
            call("GET", BASE + "/" + order_id)
        except urllib.error.HTTPError as error:
            if error.code != 404:
                raise
        else:
            raise RuntimeError("deleted order was still present")
        print("Persisted create/read/list/update/delete and same-value update passed.")
    finally:
        if not deleted:
            try:
                call("DELETE", BASE + "/" + order_id, expected=204)
            except Exception:
                # ID is generated test data, not a token or credential. Operator can remove it later.
                print("Could not clean up generated smoke order " + order_id + "; check delete grants.", file=sys.stderr)


if __name__ == "__main__":
    try:
        run()
    except Exception:
        sys.exit("CRUD smoke failed; verify readiness and read/write/delete scopes and roles.")
