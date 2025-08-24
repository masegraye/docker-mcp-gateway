console.log("Testing error handling...");

try {
  console.log("Before error");
  throw new Error("This is a test error!");
} catch (e) {
  console.log("Caught error:", e.message);
}

console.log("After error handling");

// This will cause an uncaught error and non-zero exit
setTimeout(() => {
  throw new Error("Uncaught async error!");
}, 10);