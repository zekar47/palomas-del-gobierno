#!/usr/bin/env python3
"""Convierte coverage.out (formato gocover) al formato genérico de SonarQube.

Uso: python3 scripts/gocover2sonar.py coverage.out coverage-sonar.xml

Sin dependencias externas: evita fijar el CI a un conversor de terceros.
"""
import re
import sys
import xml.etree.ElementTree as ET

LINE_RE = re.compile(r"^(.+):(\d+)\.\d+,(\d+)\.\d+ \d+ (\d+)$")


def main(src: str, dst: str) -> None:
    covered: dict[str, dict[int, bool]] = {}
    with open(src, encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line or line.startswith("mode:"):
                continue
            m = LINE_RE.match(line)
            if not m:
                continue
            path, start, end, count = m.group(1), int(m.group(2)), int(m.group(3)), int(m.group(4))
            # coverage.out usa "modulo/archivo.go"; SonarQube quiere ruta al fuente.
            if "/" in path:
                path = path.rsplit("/", 1)[1]
            hit = count > 0
            lines = covered.setdefault(path, {})
            for n in range(start, end + 1):
                lines[n] = lines.get(n, False) or hit

    root = ET.Element("coverage", version="1")
    for path in sorted(covered):
        fel = ET.SubElement(root, "file", path=path)
        for n in sorted(covered[path]):
            ET.SubElement(
                fel,
                "lineToCover",
                lineNumber=str(n),
                covered="true" if covered[path][n] else "false",
            )
    tree = ET.ElementTree(root)
    ET.indent(tree)
    tree.write(dst, encoding="utf-8", xml_declaration=True)
    print(f"escrito {dst} ({len(covered)} archivos)")


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2])
