from flask import Flask, request, jsonify
import glob
import logging
import os
import shutil

app = Flask(__name__)

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

UPLOAD_DIR = "./uploads"

@app.route("/process", methods=["POST"])
def process_file():
    data = request.json
    
    if not data or "filename" not in data:
        return jsonify({
            "error": "filename is required"
        }), 400

    # Preserve the original case for the user-facing filename; only the
    # extension needs to be normalized for category lookup.
    filename = data["filename"].strip()
    # storedAs is the on-disk filename written by the backend (timestamp-prefixed).
    # It's optional so this endpoint still works when called directly for testing.
    stored_as = (data.get("storedAs") or filename).strip()

    EXT_MAP = {
    ".jpg": "Photos",
    ".jpeg": "Photos",
    ".png": "Photos",
    ".heic": "Photos",
    ".heif": "Photos",
    ".hevc": "Photos",
    ".heif": "Photos",
    ".svg": "Photos",
    ".webp": "Photos",
    ".gif": "Photos",
    ".bmp": "Photos",
    ".tiff": "Photos",
    ".ico": "Photos",
    ".pdf": "Documents",
    ".docx": "Documents",
    ".doc": "Documents",
    ".xls": "Documents",
    ".xlsx": "Documents",
    ".pptx": "Documents",
    ".ppt": "Documents",
    ".txt": "Documents",
    ".csv": "Documents",
    ".md": "Documents",
    ".mp3": "Audio",
    ".mp4": "Video",
    ".avi": "Video",
    ".mkv": "Video",
    ".mov": "Video",
    ".wmv": "Video",
    ".flv": "Video",
    ".mpeg": "Video",
    ".mpg": "Video",
    ".m4v": "Video",
    ".m4a": "Audio",
    ".m4b": "Audio",
    ".m4p": "Audio",
    ".m4v": "Video",
    ".m4a": "Audio",
    ".m4b": "Audio",
    ".m4p": "Audio",
}

    dot = filename.rfind(".")
    ext = filename[dot:].lower() if dot != -1 else ""

    category = EXT_MAP.get(ext, "Others")

    new_name = f"{category}/{filename}"

    logger.info(f"Processing {stored_as} -> {new_name}")
    try:
        final_path = moveToCategory(stored_as, filename, category)
    except FileNotFoundError as e:
        logger.warning(str(e))
        return jsonify({
            "status": "error",
            "error": "source file not found",
            "data": {
                "original_name": filename,
                "stored_as": stored_as,
                "category": category,
            }
        }), 404

    return jsonify({
        "status": "success",
        "data": {
            "original_name": filename,
            "stored_as": stored_as,
            "category": category,
            "new_name": new_name,
            "moved": True,
            "final_path": final_path,
        }
    })

# Resolve the actual on-disk upload path. The backend writes files as
# "<unix_nano>_<original_name>", but the caller (n8n, curl, tests) may pass
# the clean name, the timestamped name, or a slightly different case. We try,
# in order:
#   1. exact match on what the caller gave us
#   2. exact match on the clean filename (caller forgot storedAs)
#   3. glob "*_<clean_name>" with case-insensitive fallback, picking the
#      most recently modified match so re-uploads resolve to the latest file.
def resolveSource(stored_as: str, clean_name: str) -> str:
    candidates = []
    if stored_as:
        candidates.append(os.path.join(UPLOAD_DIR, stored_as))
    candidates.append(os.path.join(UPLOAD_DIR, clean_name))

    for path in candidates:
        if os.path.isfile(path):
            return path

    # Fall back to scanning ./uploads for "*_<clean_name>".
    pattern = os.path.join(UPLOAD_DIR, f"*_{clean_name}")
    matches = [p for p in glob.glob(pattern) if os.path.isfile(p)]

    if not matches:
        # Case-insensitive sweep of the top-level uploads dir.
        target = clean_name.lower()
        try:
            entries = os.listdir(UPLOAD_DIR)
        except FileNotFoundError:
            entries = []
        for entry in entries:
            entry_path = os.path.join(UPLOAD_DIR, entry)
            if not os.path.isfile(entry_path):
                continue
            lower = entry.lower()
            if lower == target or lower.endswith(f"_{target}"):
                matches.append(entry_path)

    if not matches:
        raise FileNotFoundError(
            f"Source file not found for stored_as={stored_as!r}, filename={clean_name!r}"
        )

    matches.sort(key=os.path.getmtime, reverse=True)
    return matches[0]


# Move the resolved upload to {UPLOAD_DIR}/{category}/{clean_name}, renaming
# it to the clean, user-facing filename in the process. Collisions are
# resolved by appending a numeric suffix. Returns the final destination path.
def moveToCategory(stored_as: str, clean_name: str, category: str) -> str:
    src = resolveSource(stored_as, clean_name)
    dst_dir = os.path.join(UPLOAD_DIR, category)
    dst = os.path.join(dst_dir, clean_name)

    os.makedirs(dst_dir, exist_ok=True)

    # If the clean destination already exists (same-named upload in the past),
    # keep both by suffixing: name.ext, name (1).ext, name (2).ext, ...
    if os.path.exists(dst):
        base, ext = os.path.splitext(clean_name)
        i = 1
        while True:
            candidate = os.path.join(dst_dir, f"{base} ({i}){ext}")
            if not os.path.exists(candidate):
                dst = candidate
                break
            i += 1

    shutil.move(src, dst)
    logger.info(f"Moved {src} -> {dst}")
    return dst

if __name__ == "__main__":
    app.run(host="0.0.0.0", port=5000)
