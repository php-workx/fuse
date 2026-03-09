#!/usr/bin/env python3
"""Script using subprocess, eval, and exec — all dangerous."""

import subprocess
import shutil

# subprocess.Popen — spawns a child process
proc = subprocess.Popen(["python3", "-c", "print('hello')"], stdout=subprocess.PIPE)
output, _ = proc.communicate()
print(output.decode())

# subprocess.check_call
subprocess.check_call(["echo", "running check_call"])

# eval — dynamic code execution
user_input = "2 + 2"
result = eval(user_input)
print(f"Eval result: {result}")

# exec — dynamic code execution
code = "x = 10\nprint(x)"
exec(code)

# __import__ — dynamic import
mod = __import__('json')
print(mod.dumps({"key": "value"}))

# importlib for dynamic imports
import importlib
module = importlib.import_module('collections')

# shutil.rmtree — recursive directory removal
shutil.rmtree('/tmp/old_cache', ignore_errors=True)

# shutil.move — could overwrite files
shutil.move('/tmp/source', '/tmp/destination')

# os.remove
import os
os.remove('/tmp/tempfile.txt')
