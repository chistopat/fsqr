import unittest
from importlib import util
from pathlib import Path

MODULE_PATH = Path(__file__).with_name("next_release.py")
SPEC = util.spec_from_file_location("next_release", MODULE_PATH)
next_release = util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(next_release)


class NextReleaseTest(unittest.TestCase):
    def test_next_release_tag_uses_only_strict_v0_0_patch_tags(self):
        tags = ["v0.0.1", "v0.0.9", "v.0.0.10", "v0.1.0", "junk", "v0.0.alpha"]
        self.assertEqual(next_release.next_release_tag(tags), "v0.0.10")

    def test_next_release_tag_defaults_to_one_without_matching_tags(self):
        self.assertEqual(next_release.next_release_tag(["v.0.0.1", "v0.1.0"]), "v0.0.1")

    def test_ghcr_image_normalizes_repository_name(self):
        self.assertEqual(next_release.ghcr_image("Chistopat/FSQR"), "ghcr.io/chistopat/fsqr")

    def test_ghcr_image_rejects_invalid_repository_name(self):
        with self.assertRaises(ValueError):
            next_release.ghcr_image("fsqr")


if __name__ == "__main__":
    unittest.main()
