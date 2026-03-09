// A safe JavaScript script — no dangerous operations.

const data = [1, 2, 3, 4, 5];

// Array operations
const doubled = data.map(x => x * 2);
const filtered = data.filter(x => x > 3);
const total = data.reduce((acc, x) => acc + x, 0);

console.log("Doubled:", doubled);
console.log("Filtered:", filtered);
console.log("Total:", total);

// JSON parsing
const jsonStr = '{"name": "test", "values": [10, 20, 30]}';
const parsed = JSON.parse(jsonStr);
console.log("Parsed:", parsed);

// String operations
const greeting = "Hello, World!";
console.log(greeting.toUpperCase());
console.log(greeting.split("").reverse().join(""));

// Object manipulation
const config = {
    host: "localhost",
    port: 8080,
    debug: false,
};

const entries = Object.entries(config);
console.log("Config entries:", entries);

// Simple class
class Calculator {
    constructor(initial = 0) {
        this.value = initial;
    }

    add(n) {
        this.value += n;
        return this;
    }

    result() {
        return this.value;
    }
}

const calc = new Calculator(10).add(5).add(3);
console.log("Calc result:", calc.result());
