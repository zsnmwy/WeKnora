import sys
import unittest
from pathlib import Path

ROOT_DIR = Path(__file__).resolve().parents[2]
if str(ROOT_DIR) not in sys.path:
    sys.path.insert(0, str(ROOT_DIR))

from docreader.parser.registry import PDFParser, normalize_file_type, registry


class ParserEngineRegistryTest(unittest.TestCase):
    def test_normalize_file_type_strips_leading_dot(self):
        self.assertEqual(normalize_file_type(".PDF"), "pdf")

    def test_normalize_file_type_maps_mime_type(self):
        self.assertEqual(normalize_file_type("application/pdf"), "pdf")
        self.assertEqual(normalize_file_type("application/json; charset=utf-8"), "json")

    def test_get_parser_class_accepts_dotted_extension(self):
        self.assertIs(registry.get_parser_class("builtin", ".pdf"), PDFParser)


if __name__ == "__main__":
    unittest.main()
