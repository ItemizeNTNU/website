FROM node:alpine

WORKDIR /app
COPY . .
RUN npm install
RUN npm run build

CMD ["node", "__sapper__/build"]

EXPOSE 3000