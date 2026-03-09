#!/usr/bin/env python3
"""A safe data processing script — no dangerous operations."""

import math
import json

# Process a list of numbers
numbers = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10]

# Use list comprehensions
squares = [x ** 2 for x in numbers]
evens = [x for x in numbers if x % 2 == 0]

# Calculate statistics
total = sum(numbers)
average = total / len(numbers)
std_dev = math.sqrt(sum((x - average) ** 2 for x in numbers) / len(numbers))

print(f"Numbers: {numbers}")
print(f"Squares: {squares}")
print(f"Evens: {evens}")
print(f"Total: {total}, Average: {average:.2f}, StdDev: {std_dev:.2f}")

# Parse some JSON data
data = json.loads('{"name": "test", "values": [1, 2, 3]}')
result = {k: v for k, v in data.items() if k != "name"}
print(f"Filtered data: {result}")

# A simple class
class DataProcessor:
    def __init__(self, items):
        self.items = items

    def transform(self):
        return [item.upper() if isinstance(item, str) else item for item in self.items]

processor = DataProcessor(["hello", "world", 42])
print(processor.transform())
