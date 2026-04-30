# -*- coding: utf-8 -*-
"""FreeMind mind map parser.

Converts ``.mm`` XML node trees into hierarchical Markdown so the normal
document ingestion pipeline can chunk and index the content as Markdown.
"""

from __future__ import annotations

import logging
import re
import xml.etree.ElementTree as ET
from typing import Iterable, List, Optional

from docreader.models.document import Document
from docreader.parser.base_parser import BaseParser

logger = logging.getLogger(__name__)


class FreeMindParser(BaseParser):
    """Parse FreeMind ``.mm`` files into Markdown."""

    def parse_into_text(self, content: bytes) -> Document:
        try:
            root = ET.fromstring(content)
        except ET.ParseError as exc:
            raise ValueError(f"Invalid FreeMind .mm file: {exc}") from exc

        root_node = root if _local_name(root.tag) == "node" else root.find(".//node")
        if root_node is None:
            raise ValueError("Invalid FreeMind .mm file: missing root node")

        lines: List[str] = []
        self._append_node(root_node, 1, lines)
        markdown = "\n".join(lines).strip()

        logger.info(
            "Converted FreeMind file=%s into %d markdown characters",
            self.file_name,
            len(markdown),
        )
        return Document(content=markdown)

    def _append_node(self, node: ET.Element, level: int, lines: List[str]) -> None:
        title = _node_text(node)
        children = list(_iter_child_nodes(node))

        if title:
            if level == 1 or children:
                _ensure_blank(lines)
                heading_level = min(level, 6)
                lines.append(f"{'#' * heading_level} {title}")
                lines.append("")
            else:
                lines.append(f"- {title}")

            note = _richcontent_text(node, content_type="NOTE")
            if note:
                lines.append("")
                lines.append(note)
                lines.append("")

            link = _normalize_text(node.attrib.get("LINK", ""))
            if link:
                lines.append(f"链接: {link}")
                lines.append("")

        for child in children:
            self._append_node(child, level + 1, lines)

        if children and lines and lines[-1] != "":
            lines.append("")


def _local_name(tag: str) -> str:
    return tag.rsplit("}", 1)[-1]


def _iter_child_nodes(node: ET.Element) -> Iterable[ET.Element]:
    for child in list(node):
        if _local_name(child.tag) == "node":
            yield child


def _node_text(node: ET.Element) -> str:
    text = _normalize_text(node.attrib.get("TEXT", ""))
    if text:
        return text
    return _richcontent_text(node, content_type="NODE")


def _richcontent_text(node: ET.Element, content_type: str) -> str:
    for child in list(node):
        if _local_name(child.tag) != "richcontent":
            continue
        if child.attrib.get("TYPE", "").upper() != content_type:
            continue
        return _normalize_text(" ".join(child.itertext()))
    return ""


def _normalize_text(value: Optional[str]) -> str:
    if not value:
        return ""
    return re.sub(r"\s+", " ", value).strip()


def _ensure_blank(lines: List[str]) -> None:
    if lines and lines[-1] != "":
        lines.append("")
