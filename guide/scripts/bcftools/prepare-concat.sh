#!/usr/bin/env bash
set -euo pipefail

INPUT=""
PART_DIR=""
FORCE=no

usage() {
  cat <<'EOF'
用法: prepare-concat.sh --input FILE --parts DIR [--force]

按输入 VCF 的 contig 生成 Header 一致的 concat 分片和 files.list。
已有完整缓存默认复用；--force 强制重建。
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --input) INPUT="${2:-}"; shift 2 ;;
    --parts) PART_DIR="${2:-}"; shift 2 ;;
    --force) FORCE=yes; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "未知参数: $1" >&2; usage >&2; exit 2 ;;
  esac
done

[[ -f "${INPUT}" ]] || { echo "输入文件不存在: ${INPUT}" >&2; exit 2; }
[[ -n "${PART_DIR}" && "${PART_DIR}" != "/" && "${PART_DIR}" != "." ]] || {
  echo "无效分片目录: ${PART_DIR}" >&2
  exit 2
}
command -v bcftools >/dev/null || { echo "需要 bcftools" >&2; exit 127; }

INPUT_BYTES="$(wc -c < "${INPUT}" | tr -d ' ')"
cache_complete() {
  [[ "${FORCE}" == "no" && -s "${PART_DIR}/.complete" && -s "${PART_DIR}/files.list" ]] || return 1
  [[ "$(awk -F '\t' '$1 == "input_bytes" {print $2}' "${PART_DIR}/.complete")" == "${INPUT_BYTES}" ]] || return 1
  local count=0 file
  while IFS= read -r file; do
    [[ -s "${file}" ]] || return 1
    count=$((count + 1))
  done < "${PART_DIR}/files.list"
  [[ ${count} -gt 1 ]]
}

if cache_complete; then
  echo "使用已有 concat 分片: ${PART_DIR}"
  exit 0
fi

mkdir -p "$(dirname "${PART_DIR}")"
TEMP_DIR="${PART_DIR}.tmp.$$"
rm -rf "${TEMP_DIR}"
mkdir -p "${TEMP_DIR}"
trap 'rm -rf "${TEMP_DIR}"' EXIT

mapfile -t CONTIGS < <(bcftools index --stats "${INPUT}" | awk '$3 > 0 {print $1}')
[[ ${#CONTIGS[@]} -gt 1 ]] || { echo "输入中可用 contig 不足" >&2; exit 1; }

INDEX=0
for CONTIG in "${CONTIGS[@]}"; do
  INDEX=$((INDEX + 1))
  PART="${TEMP_DIR}/$(printf '%03d' "${INDEX}").vcf.gz"
  echo "生成分片 ${INDEX}/${#CONTIGS[@]}: ${CONTIG}"
  bcftools view --no-version -r "${CONTIG}" -Oz -o "${PART}" "${INPUT}"
done

rm -rf "${PART_DIR}"
mv "${TEMP_DIR}" "${PART_DIR}"
trap - EXIT

find "${PART_DIR}" -maxdepth 1 -type f -name '*.vcf.gz' -print | sort > "${PART_DIR}/files.list"
{
  printf 'field\tvalue\n'
  printf 'input\t%s\n' "${INPUT}"
  printf 'input_bytes\t%s\n' "${INPUT_BYTES}"
  printf 'input_records\t%s\n' "$(bcftools index --nrecords "${INPUT}")"
  printf 'parts\t%s\n' "${#CONTIGS[@]}"
  printf 'bcftools\t%s\n' "$(bcftools --version-only)"
} > "${PART_DIR}/.complete"

echo "concat 分片准备完成: ${PART_DIR}"
