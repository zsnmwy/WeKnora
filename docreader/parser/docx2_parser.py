import logging
import zipfile
from io import BytesIO

from docreader.parser.chain_parser import FirstParser
from docreader.parser.docx_parser import DocxParser
from docreader.parser.markitdown_parser import MarkitdownParser

logger = logging.getLogger(__name__)


class Docx2Parser(FirstParser):
    _parser_cls = (MarkitdownParser, DocxParser)

    def parse_into_text(self, content: bytes):
        if self._has_embedded_spreadsheet(content):
            logger.info("Docx2Parser: embedded spreadsheet detected, prefer DocxParser")
            return self._parsers[1].parse_into_text(content)
        return super().parse_into_text(content)

    @staticmethod
    def _has_embedded_spreadsheet(content: bytes) -> bool:
        try:
            with zipfile.ZipFile(BytesIO(content)) as zf:
                for name in zf.namelist():
                    lower = name.lower()
                    if lower.startswith("word/embeddings/") and (
                        lower.endswith(".xlsx") or lower.endswith(".xls")
                    ):
                        return True
        except Exception:
            return False
        return False


if __name__ == "__main__":
    logging.basicConfig(level=logging.DEBUG)

    your_file = "/path/to/your/file.docx"
    parser = Docx2Parser(separators=[".", "?", "!", "。", "？", "！"])
    with open(your_file, "rb") as f:
        content = f.read()

        document = parser.parse(content)
        for cc in document.chunks:
            logger.info(f"chunk: {cc}")

        # document = parser.parse_into_text(content)
        # logger.info(f"docx content: {document.content}")
        # logger.info(f"find images {document.images.keys()}")
