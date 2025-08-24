console.log("Testing timeout (should be killed after 30 seconds)...");

let count = 0;
const interval = setInterval(() => {
  count++;
  console.log(`Still running... (${count} seconds)`);
  
  if (count >= 35) {
    console.log("This should never be reached due to 30s timeout!");
    clearInterval(interval);
  }
}, 1000);