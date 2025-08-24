# Just-In-Time Containers for MCP Servers

**Initial Purpose**: The purpose of this feature is to enable the use of just-in-time containers to run MCP servers that are currently delivered as UVX or npm packages safely inside of a container. And ideally to do that in a way where we can cache the image for reuse - something like a build cache.

## 1. Rough Thoughts
*This section will capture initial ideas and insights as they come up during our discussion.*

- **Leveraging Existing Pluggable Provisioning Infrastructure**: Build on the well-defined containerization interface from pluggable-provisioning.md that already supports Docker and Kubernetes provisioners

The existing pluggable provisioning system provides a solid foundation with its unified Provisioner interface and Container Runtime abstraction. Rather than building a completely separate system, the JIT containers feature can extend this architecture by adding dynamic build capabilities. This approach ensures consistency with the existing Docker/Kubernetes provisioning patterns and maintains the same operational model users are already familiar with. The strategy pattern already in place makes this a natural extension rather than a disruptive change.

- **Dynamic Dockerfile Generation for npm/UVX Packages**: Create a system that can take an npx command and automatically generate, build, and run containerized versions with caching

This addresses a key gap in the current ecosystem where MCP servers distributed as npm packages or UVX tools lack the isolation and security benefits of containerization. The dynamic approach means we don't need to pre-build containers for every possible package - instead, we generate Dockerfiles on-demand based on the specific package and version requirements. The build cache functionality would be crucial for performance, avoiding repeated builds of the same package/version combinations while still allowing for updates and customization.

- **Docker-First Implementation Strategy**: Focus initial implementation on Docker provisioner due to registry integration complexity, deferring Kubernetes support

This pragmatic approach makes sense given the current constraints in the pluggable provisioning system where Kubernetes support is experimental and tied to Docker Desktop. The Docker provisioner already has mature registry integration, local image caching, and well-understood security boundaries. Starting with Docker allows for rapid iteration and validation of the core concept before tackling the additional complexity of multi-cluster registry coordination and Kubernetes-specific build patterns.

- **Internal Library Architecture with MCP Tool Exposure**: Implement as standalone library in mcp internal namespace, with optional MCP tool interface for AI-assisted Dockerfile construction

This dual-interface approach provides both programmatic access for automated workflows and interactive access for AI-assisted development. The internal library ensures the core functionality can be used by other parts of the system without MCP overhead, while the MCP tool interface opens up interesting possibilities for LLM-assisted container optimization, security analysis, and custom build logic. This could enable scenarios where the AI helps optimize Dockerfiles for specific use cases or adds security hardening based on package analysis.

- **Sandboxing for Build Process**: Explore containerized build environment that provides isolated development space for Dockerfile construction and Docker builds

This addresses security concerns around dynamic Dockerfile generation and builds. Rather than running `docker build` directly on the host system with potentially untrusted content, a sandboxed build environment provides isolation. The sandbox would need network access for package installation and potentially registry access, but could output build artifacts (filesystem images, tars) rather than directly populating the local Docker image cache. This creates opportunities for build artifact inspection, security scanning, and controlled deployment workflows.

- **Sandbox API Architecture**: Build a sandbox API that leverages existing container runtime to create managed build environments with mount, network, and lifecycle capabilities

The sandbox API would abstract the complexity of creating isolated build environments while using the proven container runtime infrastructure already in place. This API would handle mounting source code, providing network access for package downloads, and managing the lifecycle of build containers. The key insight is that this sandbox itself becomes a managed container that can execute `docker build` operations in isolation, potentially using techniques like Docker-in-Docker or bind-mounting the Docker socket with appropriate security controls.

- **Extended Catalog Type System**: Introduce JIT-prefixed server types (jit:npm, jit:uvx) to distinguish dynamically-built containers from static POCI/server types

This extends the existing type system cleanly without disrupting current POCI (Plain Old Container Image) and server classifications. The `jit:` prefix creates a clear namespace for just-in-time built containers while maintaining backward compatibility. This typing system would trigger the JIT build process when these server types are encountered, routing them through the dynamic build pipeline instead of direct image pulls. The subtype system (npm, uvx) allows for package-manager-specific build logic and optimization strategies.

- **Dynamic Image Registration and Catalog Linkage**: Implement metadata mapping system to associate catalog entries with successfully built images, supporting iterative build processes

This addresses the crucial problem of maintaining state between build attempts and final success. The system needs to track the relationship between a catalog entry (e.g., "fastify-mcp-server" with type "jit:npm") and its successfully built Docker image. This mapping isn't persistent storage but rather session-based metadata that handles scenarios where multiple build iterations are required before achieving a working container. The system must handle cases where builds fail, succeed, or need rebuilding due to package updates or Dockerfile refinements.

- **Iterative Build Process Support**: Handle multiple build rounds with temporary image storage and metadata tracking until successful container deployment

This recognizes that JIT container building is inherently iterative - the first generated Dockerfile might not work perfectly, packages might have unexpected dependencies, or security hardening might require multiple attempts. The system needs to manage multiple image versions (attempt 1, 2, 3) while maintaining clear linkage to the originating catalog entry. Only when a build succeeds and passes validation tests should it become the "active" image for that catalog entry, with previous iterations either cleaned up or retained for debugging purposes.

- **Catalog Structure Integration**: Extend existing catalog schema to support JIT entries with package-specific metadata while maintaining compatibility with current server/POCI structure

Looking at the existing catalog structure in docker-mcp.yaml, both `type: server` and `type: poci` entries follow consistent patterns with fields like image, tools, secrets, env, config, etc. The JIT types (`jit:npm`, `jit:uvx`) would need to fit this same schema while adding package-specific fields. For npm packages, this might include package name, version constraints, and build-time dependencies. The challenge is maintaining the same rich metadata structure (tools, secrets, env variables) while replacing the static `image` field with dynamic build instructions.

- **Package-Specific Build Templates**: Develop specialized Dockerfile generation for different package ecosystems (npm global installs, uvx Python tools, etc.)

Each package ecosystem has distinct patterns - npm packages often need global installation (`npm install -g package-name`), specific Node.js versions, and may require additional system dependencies. UVX tools have their own installation patterns and Python environment requirements. The build template system needs to understand these patterns while allowing for customization and optimization. This could include base image selection (alpine vs ubuntu), package manager choices, security hardening steps, and runtime optimization.

- **E2B-Style API-Based Sandbox Model**: Leverage E2B's open-source enterprise-grade cloud infrastructure approach for secure JIT container builds with instant disposable environments

E2B (e2b.dev) offers a compelling reference architecture with ephemeral "virtual personal computers in the cloud" that spin up in under 200ms with complete isolation. Used by Fortune 100 companies and major AI companies like Hugging Face and Perplexity, their model provides instant, disposable cloud machines with fine-grained resource controls (CPU, memory, execution time limits) and automatic shutdown for misbehaving processes. For JIT containers, we could adopt a similar approach where each npm/UVX package build gets its own ephemeral sandbox environment with deterministic, reproducible builds. The key advantages include: (1) Zero-trust security with full isolation, (2) Automatic resource limits preventing runaway builds, (3) Clean environment for each build ensuring reproducibility, (4) Open-source foundation allowing self-hosted deployment. This addresses both build isolation and supply chain security concerns inherent in dynamic containerization of potentially untrusted packages.

- **Dev Containers Integration Model**: Support VS Code-style dev container workflows where JIT-built MCP servers can be developed and tested in consistent, reproducible environments

Dev containers offer a compelling developer experience where a simple `devcontainer.json` file defines the complete development environment. For JIT containers, we could support a mode where developers can iterate on MCP server packaging by working within a dev container that has the JIT build tools pre-installed. This would enable rapid prototyping of Dockerfile generation logic and testing of package-specific build templates. The Docker Compose integration could be particularly valuable for complex MCP servers that need supporting services (databases, APIs) during development and testing phases.

- **Open Interpreter Security Model**: Implement human-in-the-loop confirmation for JIT builds with optional cloud sandbox execution for untrusted package analysis

Open Interpreter's approach of requiring explicit user confirmation before executing potentially risky operations provides a good security pattern for JIT container builds. Since we're dynamically generating Dockerfiles and executing builds based on potentially untrusted npm/UVX packages, having a confirmation step before build execution could prevent supply chain attacks. The integration with E2B for Python sandboxing also suggests we could offer a "secure build mode" where Dockerfile generation and initial package analysis happens in a cloud sandbox before the actual build process begins.

- **Multi-Tier Sandbox Architecture**: Support different isolation levels from lightweight containers to full virtual desktop instances based on security requirements and use cases

The research reveals a spectrum of sandbox approaches from containers (lightweight, process-level isolation) to full VMs/VDI (hardware-level isolation with GUI support) to simulation environments (domain-specific 3D worlds). For JIT containers, we should consider supporting multiple tiers: (1) Standard Docker builds for trusted packages, (2) Containerized build environments for moderate security needs, (3) MicroVM or full VM builds for high-security scenarios, and potentially (4) Virtual desktop instances for MCP servers that need GUI interaction or complex system access. Each tier offers different trade-offs between security, performance, and capability.

- **Layered Implementation Strategy**: Start with the narrowest, highest-level use case to minimize surface area, then expand underlying platform layers incrementally to support additional use cases

The question of whether to start at the bottom (platform layers) or top (specific use cases) is critical for managing complexity and development risk. While technically the bottom layers determine the capabilities of upper layers, starting at the top with the most focused use case allows us to build only the platform capabilities needed for that specific scenario. For example, we could begin with just lightweight container-based sandboxes for evaluating npm package code - this would require us to build out a version of all the architectural layers (sandbox API, build orchestration, image caching, catalog integration) but in their simplest form. Once that works, we could expand to support dev container-style workflows, which might share the same underlying platform layers but require different customer APIs and development tooling. The third expansion could add strong isolation via microVMs, requiring enhancements to the platform layers but leveraging the existing API patterns. Finally, full virtual desktop instances might be architecturally different enough to warrant a separate implementation entirely. This approach minimizes initial surface area while creating a foundation that can be expanded incrementally based on validated use cases and customer feedback.

- **Sandbox Capability Layering**: Structure sandbox offerings in increasing capability and complexity tiers, each building upon the previous layer's infrastructure

From a technical architecture perspective, the sandbox capabilities would layer as follows: **Layer 1a - Raw Code Evaluation**: Execute arbitrary JavaScript or Python code snippets in a sandbox with only built-in/standard libraries available. No external dependencies, no package management - just evaluate the code you provide with whatever's in the base runtime. **Layer 1b - Code with Dependencies**: Execute code snippets but allow specifying dependencies that get dynamically installed. Behind the scenes we generate package.json/requirements.txt and handle the dependency resolution and installation process. **Layer 1c - Package Command Execution**: Execute npm scripts, Python modules, or full package commands (like `npx create-react-app` or `python -m pytest`). This involves taking whole npm packages or Python packages and running their intended commands/scripts. **Layer 2 - Dev Container Workflows**: Enhanced container environments with persistent volumes, Docker Compose support, and development tooling integration. Builds on Layer 1's container runtime but adds orchestration capabilities. **Layer 3 - MicroVM Isolation**: Hardware-level isolation using Firecracker or similar technologies for high-security scenarios. Requires new runtime infrastructure but can leverage Layer 1's API patterns. **Layer 4 - Full Virtual Desktops**: Complete OS instances with GUI support for complex MCP servers needing system-level access. May require entirely separate infrastructure due to fundamentally different resource and networking requirements. Each layer serves different customer segments and complexity levels, with Layer 1a being the absolute minimal surface area to start with.

- **Progressive API Complexity**: Design the sandbox API to start with the simplest possible interface and expand capabilities incrementally while maintaining backward compatibility

The API evolution would mirror the capability layers: **API v1a** could be as simple as `POST /sandbox/eval` with just `{code: "console.log('hello')", language: "javascript"}`. This requires minimal infrastructure - just a Node.js or Python runtime in a container. **API v1b** expands to include dependencies: `{code: "...", language: "javascript", dependencies: ["axios", "lodash"]}`. Behind the scenes, this triggers dynamic package.json generation and npm install before code execution. **API v1c** adds package execution: `{package: "create-react-app", command: "my-app", args: ["--template", "typescript"]}`. This is where we transition toward the JIT container use case, as we're now executing arbitrary npm packages that might become MCP servers. The key insight is that each API version builds on the previous infrastructure while adding just enough new capability to serve the next use case. This allows us to validate demand and usage patterns at each level before investing in the more complex underlying platform capabilities needed for the next tier.

- **Native TypeScript Support in Node.js v23.6.0+**: Leverage the new built-in TypeScript support to simplify Layer 1a implementation without requiring additional tooling or compilation steps

Node.js v23.6.0 (released January 2025) introduced native TypeScript support, allowing direct execution of `.ts`, `.mts`, and `.cts` files without dependencies like `ts-node` or manual compilation. This significantly simplifies our Layer 1a implementation since we can execute TypeScript code directly using `node script.ts` without any build steps. The official Docker images are already available as `node:23.6.0`, `node:23.6.0-slim`, `node:23.6.0-alpine`, etc., maintained by the Node.js organization on Docker Hub. For our minimal sandbox API, this means we could support both JavaScript and TypeScript code evaluation in Layer 1a using the same base image and execution model. This eliminates the complexity of choosing between JavaScript-only execution or adding TypeScript compilation infrastructure, making the initial implementation even simpler while providing broader language support from day one.

- **Layer 1a Implementation Complete**: Built and deployed working prototype of base sandbox with raw code evaluation capabilities

**Built Images** (temporary - will be replaced with official images later):
- `masongraye827/gw-sandbox-base-node:v23.6.0` - Layer 1a implementation with JavaScript and TypeScript support
- `masongraye827/gw-sandbox-base-node:v23` - Latest minor build of the v23 series

**Implementation Details**:
- Go HTTP server providing `/eval` and `/health` endpoints with 30-second timeout protection
- Multi-stage Docker build using Node.js v23.6.0-alpine for native TypeScript support
- Security hardening: non-root user, resource limits, capability dropping, read-only filesystem
- Complete testing infrastructure with helper scripts for easy API interaction
- Located at: `tools/sandboxing/sandbox-images/base-sandbox-node@v23.6.0/`

This prototype validates the Layer 1a architecture and provides a foundation for building Layer 1b (code with dependencies) and subsequent layers. The implementation demonstrates the minimal surface area approach while providing both JavaScript and TypeScript execution capabilities in a secure, containerized environment.

## 2. Analysis
*This section will organize and examine the rough thoughts more systematically.*

## 3. Distillation
*This section will extract the core insights and key principles from the analysis.*

## 4. Proposed Outline
*This section will structure the key findings into a coherent framework.*

## 5. Distillation and Thesis
*This section will present the refined thesis and core principles.*

## 6. Draft
*This section will contain the final working draft of the document.*