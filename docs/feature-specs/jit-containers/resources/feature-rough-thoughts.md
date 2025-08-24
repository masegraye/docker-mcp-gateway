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