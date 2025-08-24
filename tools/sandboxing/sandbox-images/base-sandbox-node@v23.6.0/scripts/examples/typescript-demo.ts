interface Person {
  name: string;
  age: number;
}

const person: Person = {
  name: "Alice",
  age: 30
};

console.log("Hello from TypeScript!");
console.log(`Person: ${person.name}, Age: ${person.age}`);

function fibonacci(n: number): number {
  if (n <= 1) return n;
  return fibonacci(n - 1) + fibonacci(n - 2);
}

console.log("Fibonacci sequence:");
for (let i = 0; i < 8; i++) {
  console.log(`fib(${i}) = ${fibonacci(i)}`);
}