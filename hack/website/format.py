import re
import os
import ast
import sys
import json


def convert_link(md):
    data = ""
    with open(md, "r") as f:
        data = f.read()
        url_arr = re.findall(r'(\[(.*?)\]\((.*?)\))', data)
        for url in url_arr:
            if url[2].startswith("http") or url[2].startswith("https"):
                continue
            if ".md" in url[2] or ".mdx" in url[2]:
                new_path = url[2].replace(".mdx", "")
                new_path = new_path.replace(".md", "")
                new_url = "[{}]({})".format(url[1], new_path)
                data = data.replace(url[0], new_url)
                print(f"convert {url[0]} to {new_url}")

    with open(md, "w") as f:
        f.write(data)


def format_markdown(path):
    files = os.walk(path)
    for path, dir_list, file_list in files:
        for file_name in file_list:
            file_path = os.path.join(path, file_name)
            if file_path[-3:] == ".md" or file_path[-4:] == ".mdx":
                convert_link(file_path)


def format_json(file):
    data = ""
    with open(file, "r") as f:
        content = f.read()
        data = ast.literal_eval(re.sub(r"(\w+)=", r"'\1':", content))
        print(f"format versions.json: {data}")

    with open(file, "w") as f:
        f.write(json.dumps(data))


if __name__ == "__main__":
    if len(sys.argv) != 3:
        sys.exit(1)

    fmt_type = sys.argv[1]
    path = sys.argv[2]
    if fmt_type == "markdown":
        format_markdown(path)

    if fmt_type == "json":
        format_json(path)
