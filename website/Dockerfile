# Stage 1: Build SolidJS web client
FROM node:20-alpine AS webclient-builder
WORKDIR /webclient
COPY web-client/package*.json web-client/bun.lock ./
RUN npm ci
COPY web-client/ ./
RUN VITE_BASE_PATH=/app/ npm run build

# Stage 2: Build website
FROM node:20-alpine AS builder
WORKDIR /app
COPY website/package*.json ./
RUN npm ci
COPY website/ ./
COPY docs /docs
COPY --from=webclient-builder /webclient/dist ./public/app/
RUN npm run build

# Stage 3: Production
FROM nginx:alpine
COPY --from=builder /app/dist /usr/share/nginx/html
COPY website/nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
