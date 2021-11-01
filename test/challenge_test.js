const { expect } = require("chai");
const fs = require("fs");


// golden minigeth.bin hash
const goldenRoot = "0xff382489fd6109b9dc2dcee3f6e202e6ae2a5e7029cd9eb4b1f931dff51ca725"

describe("Challenge contract", function () {
  beforeEach(async function () {
    // this mips can be reused for other challenges
    const MIPS = await ethers.getContractFactory("MIPS")
    const m = await MIPS.deploy()
    mm = await ethers.getContractAt("MIPSMemory", await m.m())

    const Challenge = await ethers.getContractFactory("Challenge")
    c = await Challenge.deploy(m.address, goldenRoot)
  })
  it("challenge contract deploys", async function() {
    console.log("Challenge deployed at", c.address)
  })
  it("initiate challenge", async function() {
    // TODO: is there a better way to get the "HardhatNetworkProvider"?
    const hardhat = network.provider._wrapped._wrapped._wrapped._wrapped._wrapped
    const blockchain = hardhat._node._blockchain

    // get data
    const blockNumberN = (await ethers.provider.getBlockNumber())-1;
    const blockNp1 = blockchain._data._blocksByNumber.get(blockNumberN+1)
    const blockNp1Rlp = blockNp1.header.serialize()

    const assertionRoot = "0x9e0261efe4509912b8862f3d45a0cb8404b99b239247df9c55871bd3844cebbd"

    const finalSystemState = "0xa9aaac45d9ccaeab0b97eff2d7ce6050948f8322c869c0a8f94a0d7013c31824"
    let startTrie = JSON.parse(fs.readFileSync("/tmp/eth/13284469/checkpoint_-1.json"))
    let finalTrie = JSON.parse(fs.readFileSync("/tmp/eth/13284469/checkpoint_85042025.json"))

    while (1) {
      try {
        await c.InitiateChallenge(blockNumberN, blockNp1Rlp, assertionRoot, finalSystemState, 1)
      } catch(e) {
        const missing = e.toString().split("'")[1]
        if (missing.length == 64) {
          console.log("requested node", missing)
          let node = startTrie['preimages']["0x"+missing]
          if (node === undefined) {
            node = finalTrie['preimages']["0x"+missing]
          }
          expect(node).to.not.be.an('undefined')
          const bin = Uint8Array.from(Buffer.from(node, 'base64').toString('binary'), c => c.charCodeAt(0))
          await mm.AddTrieNode(bin)
          continue
        } else {
          console.log(e)
          break
        }
      }
    }

    //const blockHeaderNp1 = getBlockRlp(await ethers.provider.getBlock(blockNumberN+1));
    //console.log(blockNumberN, blockHeaderNp1);
  })
})