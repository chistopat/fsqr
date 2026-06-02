#!/usr/bin/env python3
"""Compute the next fsqr release tag and GHCR image name for GitHub Actions."""

from __future__ import annotations

import argparse
import os
import re
import subprocess
from pathlib import Path
from typing import Iterable, Sequence

TAG_RE = re.compile(r"^v0\.0\.(\d+)$")


def run_git(args: Sequence[str]) -> str:
    result = subprocess.run(
        ["git", *args],
        check=True,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    return result.stdout.strip()


def strict_release_patches(tags: Iterable[str]) -> list[int]:
    patches: list[int] = []
    for tag in tags:
        match = TAG_RE.fullmatch(tag.strip())
        if match is not None:
            patches.append(int(match.group(1)))
    return patches


def next_release_tag(tags: Iterable[str]) -> str:
    patches = strict_release_patches(tags)
    latest_patch = max(patches, default=0)
    return f"v0.0.{latest_patch + 1}"


def ghcr_image(repository: str) -> str:
    normalized = repository.strip().lower()
    if not normalized or "/" not in normalized:
        raise ValueError("repository must use the owner/name form")
    return f"ghcr.io/{normalized}"


def write_github_output(path: str | None, values: dict[str, str]) -> None:
    lines = [f"{key}={value}" for key, value in values.items()]
    if path:
        with Path(path).open("a", encoding="utf-8") as output:
            for line in lines:
                output.write(line + "\n")
        return

    for line in lines:
        print(line)


def compute_release(fetch_tags: bool, repository: str) -> dict[str, str]:
    if fetch_tags:
        run_git(["fetch", "--force", "--tags", "origin"])

    tags = run_git(["tag", "--list", "v0.0.*"]).splitlines()
    return {
        "version": next_release_tag(tags),
        "image": ghcr_image(repository),
    }


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Compute next fsqr release outputs for GitHub Actions.")
    parser.add_argument(
        "--repository",
        default=os.environ.get("GITHUB_REPOSITORY", ""),
        help="GitHub repository in owner/name form. Defaults to GITHUB_REPOSITORY.",
    )
    parser.add_argument(
        "--no-fetch",
        action="store_true",
        help="Do not fetch remote tags before computing the next version.",
    )
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    outputs = compute_release(fetch_tags=not args.no_fetch, repository=args.repository)
    write_github_output(os.environ.get("GITHUB_OUTPUT"), outputs)


if __name__ == "__main__":
    main()
