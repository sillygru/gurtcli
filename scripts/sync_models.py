#!/usr/bin/env python3
"""
Sync models from an OpenAI-compatible API to llm/llmdetails.json.

Prompts for API endpoint URL and key, fetches models, merges them
into the JSON file, and reports what changed.

Usage:
    python scripts/sync_models.py
"""

import json
import os
import ssl
import sys
import urllib.error
import urllib.request
from datetime import datetime, timezone

JSON_PATH = os.path.join(
    os.path.dirname(os.path.dirname(os.path.abspath(__file__))),
    "llm",
    "llmdetails.json",
)

DEFAULT_PROVIDERS = ["OpenAI", "Anthropic", "Gemini", "Others"]


# ---------------------------------------------------------------------------
# helpers
# ---------------------------------------------------------------------------

def load_json():
    with open(JSON_PATH, "r") as f:
        return json.load(f)


def _compact(obj):
    """Serialize a model dict as a single-line JSON string."""
    return json.dumps(obj, ensure_ascii=False, separators=(",", ":"))


def _format_json(data):
    """Serialize the full JSON with each model entry on its own line."""
    lines = ["{"]
    providers = list(data.keys())
    for pi, prov in enumerate(providers):
        lines.append(f'  {json.dumps(prov)}: {{')
        models = data[prov].get("data", [])
        lines.append("    \"data\": [")
        for mi, m in enumerate(models):
            comma = "," if mi < len(models) - 1 else ""
            lines.append(f"      {_compact(m)}{comma}")
        lines.append("    ]")
        comma = "," if pi < len(providers) - 1 else ""
        lines.append(f"  }}{comma}")
    lines.append("}")
    return "\n".join(lines) + "\n"


def save_json(data):
    tmp = JSON_PATH + ".tmp"
    with open(tmp, "w") as f:
        f.write(_format_json(data))
    os.replace(tmp, JSON_PATH)


_KNOWN_UC = {"gpt", "glm", "ai"}


def make_display_name(model_id):
    parts = model_id.replace("_", "-").split("-")
    result = []
    for p in parts:
        if not p:
            continue
        if p[0].isdigit() or "." in p:
            result.append(p)
        elif p.lower() in _KNOWN_UC:
            result.append(p.upper())
        else:
            result.append(p[0].upper() + p[1:])
    return " ".join(result)


def timestamp_to_iso(ts):
    return datetime.fromtimestamp(ts, tz=timezone.utc).strftime(
        "%Y-%m-%dT%H:%M:%SZ"
    )


# ---------------------------------------------------------------------------
# API call
# ---------------------------------------------------------------------------

def fetch_models(base_url, api_key):
    url = base_url.rstrip("/") + "/models"
    req = urllib.request.Request(url)
    req.add_header("Authorization", f"Bearer {api_key}")
    req.add_header("Content-Type", "application/json")
    ctx = ssl.create_default_context()

    try:
        resp = urllib.request.urlopen(req, context=ctx, timeout=30)
    except urllib.error.HTTPError as e:
        body = e.read().decode(errors="replace")[:500]
        if e.code == 401:
            print("  Error: API key rejected (HTTP 401)")
        elif e.code == 403:
            print("  Error: Forbidden (HTTP 403)")
        else:
            print(f"  Error HTTP {e.code}: {body}")
        return None
    except urllib.error.URLError as e:
        print(f"  Error: {e.reason}")
        return None
    except OSError as e:
        print(f"  Error: {e}")
        return None

    try:
        body = json.loads(resp.read().decode())
    except json.JSONDecodeError as e:
        print(f"  Error: invalid JSON response: {e}")
        return None

    if isinstance(body, dict):
        for key in ("data", "models", "results"):
            if key in body and isinstance(body[key], list):
                return body[key]
        return None
    if isinstance(body, list):
        return body

    print(f"  Error: unexpected response shape")
    return None


# ---------------------------------------------------------------------------
# merging
# ---------------------------------------------------------------------------

def deep_merge(existing, new, prefix=""):
    changed = set()
    for key, val in new.items():
        full = f"{prefix}.{key}" if prefix else key
        if isinstance(val, dict) and isinstance(existing.get(key), dict):
            changed |= deep_merge(existing[key], val, prefix=full)
        elif existing.get(key) != val:
            existing[key] = val
            changed.add(full)
    return changed


def merge_model(existing, api_model):
    """Deep-merge API model fields into the existing JSON entry.

    Fields the API doesn't include are left untouched.
    Returns a set of changed field paths (e.g. ``{"created_at"}``,
    ``{"capabilities.batch"}``).
    """
    changed = set()

    for key, val in api_model.items():
        if key in ("id", "owned_by"):
            continue  # matched against already; owned_by is not in the schema
        if key == "object":
            json_key, mapped = "type", val
        elif key == "created":
            if not isinstance(val, (int, float)):
                continue
            json_key, mapped = "created_at", timestamp_to_iso(val)
        else:
            json_key, mapped = key, val

        if isinstance(mapped, dict) and isinstance(existing.get(json_key), dict):
            sub = deep_merge(existing[json_key], mapped)
            if sub:
                changed.add(json_key)
        elif existing.get(json_key) != mapped:
            existing[json_key] = mapped
            changed.add(json_key)

    return changed


# ---------------------------------------------------------------------------
# provider selection for new models
# ---------------------------------------------------------------------------

def _owner_hint(owned_by):
    if not owned_by:
        return None
    low = owned_by.lower()
    mapping = {
        "openai": "OpenAI",
        "openai-internal": "OpenAI",
        "anthropic": "Anthropic",
        "google": "Gemini",
        "googleapis": "Gemini",
        "gemini": "Gemini",
    }
    return mapping.get(low)


def choose_provider(data, owned_by, model_id):
    print(f"\n  Model '{model_id}' not found in any existing group.")

    existing_keys = [k for k in data if k]
    if not existing_keys:
        existing_keys = DEFAULT_PROVIDERS[:]

    hint = _owner_hint(owned_by) or "Others"

    print(f"  Existing groups: {', '.join(existing_keys)}")
    print(f"  Enter a group name, or 'new:<Name>' to create one, or press Enter for [{hint}]")

    choice = input("  Group: ").strip()
    if not choice:
        return hint
    if choice.lower().startswith("new:"):
        name = choice[4:].strip()
        if name not in data:
            data[name] = {"data": []}
        return name
    for k in data:
        if k.lower() == choice.lower():
            return k
    print(f"  Group '{choice}' not found — using '{hint}'.")
    return hint


# ---------------------------------------------------------------------------
# provider index
# ---------------------------------------------------------------------------

def build_index(data):
    index = {}
    for provider, content in data.items():
        if not isinstance(content, dict):
            continue
        for i, m in enumerate(content.get("data", [])):
            mid = m.get("id")
            if mid:
                index[mid] = (provider, i)
    return index


# ---------------------------------------------------------------------------
# main loop
# ---------------------------------------------------------------------------

def main():
    if not os.path.isfile(JSON_PATH):
        print(f"Error: {JSON_PATH} not found.")
        sys.exit(1)

    data = load_json()
    print(f"Loaded {JSON_PATH}")

    while True:
        print()
        base_url = input("API endpoint URL (e.g. https://api.openai.com/v1): ").strip()
        if not base_url:
            continue

        api_key = input("API key: ").strip()
        if not api_key:
            continue

        print(f"\n  Fetching models from {base_url} ...")
        api_models = fetch_models(base_url, api_key)
        if api_models is None:
            choice = input("\n  Try again? (Y/n): ").strip().lower()
            if choice != "n":
                continue
            break

        # remove entries without an id
        api_models = [m for m in api_models if m.get("id")]
        print(f"  Received {len(api_models)} model(s).")

        index = build_index(data)
        added = []
        updated = []
        matched = 0

        for m in api_models:
            mid = m["id"]
            if mid in index:
                matched += 1
                provider, idx = index[mid]
                existing = data[provider]["data"][idx]
                changed = merge_model(existing, m)
                if changed:
                    updated.append((provider, mid, sorted(changed)))
            else:
                provider = choose_provider(data, m.get("owned_by"), mid)
                entry = {"type": "model", "id": mid}
                entry["display_name"] = make_display_name(mid)

                created = m.get("created")
                if isinstance(created, (int, float)):
                    entry["created_at"] = timestamp_to_iso(created)

                for k, v in m.items():
                    if k in ("id", "object", "created"):
                        continue
                    if k == "type":
                        continue
                    if k == "owned_by":
                        continue
                    if k not in entry:
                        entry[k] = v

                if provider not in data:
                    data[provider] = {"data": []}
                data[provider]["data"].append(entry)
                added.append((provider, mid))

        save_json(data)

        print(f"\n  --- Summary ---")
        print(f"  API models processed: {len(api_models)}")
        print(f"  Already known:        {len(api_models) - len(added)}")
        print(f"  Fields updated:       {len(updated)}")
        for provider, mid, fields in updated:
            print(f"    ~ [{provider}] {mid}: {', '.join(fields)}")
        print(f"  New models added:     {len(added)}")
        for provider, mid in added:
            print(f"    + [{provider}] {mid}")

        choice = input("\n  Add another endpoint? (Y/n): ").strip().lower()
        if choice == "n":
            break

    print("  Done.")


if __name__ == "__main__":
    main()
