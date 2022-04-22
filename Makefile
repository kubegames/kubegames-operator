NAME=ALL
PUSH=false
DEPLOY=true

install:
	cd ./cmd && ./build.sh $(NAME) $(PUSH) $(DEPLOY)

clean:
	./build/clean.sh
proto:
	./build/clean.sh
	./build/build.sh

